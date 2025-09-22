import requests
from lxml import etree
import os
import sys
import json
import time
import pprint
import kubernetes
import stat, shlex, subprocess
from kubernetes.client.rest import ApiException

# Load Kubernetes configuration (kubeconfig or in-cluster)
try:
    kubernetes.config.load_kube_config('config')
except Exception:
    kubernetes.config.load_incluster_config()

# Kubernetes CoreV1 API for PVC/PV
api_instance = kubernetes.client.CoreV1Api()

# QNAP server credentials from environment variables
qnap_ip = os.environ.get("IP")
username = os.environ.get("USERNAME")
password = os.environ.get("PASSWORD")

if not (qnap_ip and username and password):
    print("請設定環境變數 IP, USERNAME, PASSWORD。")
    sys.exit(1)

# 1) 登入 QNAP 並取得 SID
while True:
    try:
        login_url = f"http://{qnap_ip}:8080/cgi-bin/filemanager/authLogin.cgi?user={username}&plain_pwd={password}"
        response = requests.get(login_url)
        break
    except Exception as e:
        print(f"登入時發生錯誤: {e}，1秒後重試...")
        time.sleep(1)

if response.status_code != 200:
    print("無法登入 QNAP 伺服器")
    sys.exit(1)

# 解析 XML 取得 authSid
root = etree.fromstring(response.content)
if root.findtext("authPassed") != '1':
    print("QNAP 身份驗證失敗")
    sys.exit(1)
auth_sid = root.findtext("authSid")
print("獲取到 SID:", auth_sid)

# 驗證 SID
check_sid_url = f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi?func=check_sid&sid={auth_sid}"
check_res = requests.get(check_sid_url).json()
if check_res.get("status") != 1:
    print("無效的 SID")
    sys.exit(1)

# 2) 定義輔助函數

def get_pvc_path(namespace, pvcname):
    try:
        pvc = api_instance.read_namespaced_persistent_volume_claim(pvcname, namespace)
        pv_name = pvc.spec.volume_name
        pv = api_instance.read_persistent_volume(pv_name)
        nfs = pv.spec.nfs
        return nfs.path, nfs.server
    except ApiException as e:
        print(f"讀取 PVC {pvcname} 時出錯: {e}")
        return None, None


def print_progress(label, value, suffix=""):
    print(f"{label:<25}: {value:<10} {suffix}")


def track_copy_progress(filename, pid, start, amount):
    """
    持續查詢 daemon_list，根據 'pid' 更新複製進度，直到百分比達到 100
    """
    while True:
        daemon_url = (
            f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi?"
            f"func=daemon_list&subfunc=list&type=11&sid={auth_sid}"
        )
        try:
            tasks = requests.get(daemon_url).json().get("datas", [])
            for task in tasks:
                if task.get("type") != 11 or task.get("pid") != pid:
                    continue
                pct = float(task.get("percent", 0))
                prog = start + amount * pct / 100

                # —— 新增：把 prog 寫入隱藏檔 .cloneprogress
                try:
                    with open(f"/home/jovyan/.cloneprogress", "w") as f:
                        f.write(f"{prog:.1f}")
                except Exception as e:
                    print(f"寫入 .cloneprogress 時發生錯誤: {e}")

                # —— 新增：重建 current_clone_progress_is_{prog} 目錄
                #    先移除舊的，再用最新的 prog（取整數）當名稱
                os.system(f"cd /home/jovyan/ ; rm -rf current_clone_progress_is_*; mkdir current_clone_progress_is_{int(prog)}")

                # 印出進度
                print(f"\r複製中 `{filename}` - {prog:5.1f}% ", end="")
            return
        except Exception as e:
            print(f"查詢 daemon_list 時發生錯誤: {e}")
            return

# 3) 從命令列引數取得 namespace, source PVC, destination PVC
if len(sys.argv) < 4:
    print("用法: python script.py <namespace> <source_pvc> <dest_pvc>")
    sys.exit(1)

namespace = sys.argv[1]
source_pvc = sys.argv[2]
dest_pvc = sys.argv[3]

# 獲取 NFS 路徑與主機
source_path, source_host = get_pvc_path(namespace, source_pvc)
dest_path, dest_host = get_pvc_path(namespace, dest_pvc)

if not source_path or not dest_path:
    print("來源或目的 PVC 路徑讀取失敗")
    sys.exit(1)

# 如果 NFS 主機不一致，直接進行本地檔案系統複製後退出
if source_host != dest_host:
    print(f"主機不同 (source: {source_host}, dest: {dest_host})，執行本地複製。")
    # 複製一般檔／目錄
    os.system(f"cp -a {source_path}/* {dest_path}/ 2>/dev/null || true")
    os.system(f"cp -a {source_path}/.[!.]* {dest_path} 2>/dev/null || true")
    os.system(f"cp -a {source_path}/..?*  {dest_path} 2>/dev/null || true")
    sys.exit(0)

# 4) 取得來源目錄檔案列表
list_url = (
    f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi?"
    f"func=get_list&sid={auth_sid}&path={source_path}&list_mode=all&start=0&sort=filename&limit=1000&show_hidden=1"
)
files = requests.get(list_url).json().get("datas", [])
filenames = [f["filename"] for f in files]

# 5) 執行檔案複製並追蹤進度

os.system(f"find /source -type d -name '.*' ! -name '.' ! -name '..' > /tmp/list.txt")
with open("/tmp/list.txt", "r") as f:
    hidden_dirs = [d for d in f.read().splitlines() if d]

total = len(filenames) + len(hidden_dirs)
for idx, fname in enumerate(filenames):
    print(f"開始複製文件: {fname}")
    copy_url = (
        f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi?"
        f"func=copy&sid={auth_sid}&source_file={fname}&source_total=1&"
        f"source_path={source_path}&dest_path={dest_path}&mode=1"
    )
    res = requests.get(copy_url)
    print_progress("複製 API 回應", res.status_code, res.reason)
    pid = res.json().get("pid")
    if pid:
        start = 100.0 * idx / total
        amt = 100.0 / total
        track_copy_progress(fname, pid, start, amt)
    time.sleep(1)

# 6) 複製隱藏目錄 (.開頭)

def has_unsupported_nodes(root_dir: str) -> bool:
    """偵測是否含 socket/FIFO/裝置檔（File Station 會失敗的型別）"""
    for r, dirs, files in os.walk(root_dir):
        for name in files:
            p = os.path.join(r, name)
            try:
                st = os.lstat(p)
            except Exception:
                return True
            m = st.st_mode
            if stat.S_ISSOCK(m) or stat.S_ISFIFO(m) or stat.S_ISCHR(m) or stat.S_ISBLK(m):
                return True
        # 也把 .gnupg/S.gpg-agent* 這類常見 socket 規避一下（保險）
        for name in ("S.gpg-agent", "S.gpg-agent.ssh", "S.gpg-agent.extra"):
            if os.path.exists(os.path.join(r, name)):
                return True
    return False

def ensure_dirs_on_qnap(full_path: str):
    """在 QNAP 端逐層建立目錄（已存在就略過）"""
    parts = [p for p in full_path.strip("/").split("/") if p]
    if not parts:
        return
    base = "/" + parts[0]
    for seg in parts[1:]:
        try:
            requests.get(
                f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi",
                params={"func": "create_folder", "sid": auth_sid, "dest_path": base, "dest_folder": seg},
                timeout=30
            )
        except Exception:
            pass
        base = base + "/" + seg

for d in hidden_dirs:
    ff = d.split('/')
    base = ff[-1]
    rel = '/'.join(ff[2:-1])
    parent_src_abs = os.path.dirname(d)
    sp = os.path.normpath(os.path.join(source_path, rel)) if rel != "." else os.path.normpath(source_path)
    dp = os.path.normpath(os.path.join(dest_path,   rel)) if rel != "." else os.path.normpath(dest_path)
    print(f"開始複製隱藏目錄: {d} -> {dp}")
    ensure_dirs_on_qnap(dp)

    if has_unsupported_nodes(d):
        # Fallback：用 tar 打包（tar 會跳過 socket），搬到目的端後解壓
        tar_name = f"{base}.tar.gz"
        tar_src_abs = os.path.join(parent_src_abs, tar_name)
        # 打包：在來源上層 -C parent ，僅打包該資料夾
        try:
            subprocess.run(
                ["tar", "-czf", tar_src_abs,
                 "-C", parent_src_abs,
                 "--exclude=**/S.gpg-agent*", "--exclude=**/keyring-*/control",
                 "--exclude=**/*.lock", "--exclude=**/tmp/*",
                 base],
                check=True
            )
        except subprocess.CalledProcessError as e:
            print(f"打包 {d} 失敗：{e}")
            continue

    copy_url = (
        f"http://{qnap_ip}:8080/cgi-bin/filemanager/utilRequest.cgi?"
        f"func=copy&sid={auth_sid}&source_file={base}&source_total=1&"
        f"source_path={sp}&dest_path={dp}&mode=1"
    )
    res = requests.get(copy_url)
    print_progress("複製 API 回應", res.status_code, res.reason)
    pid = res.json().get("pid")
    if pid:
        start += amt
        track_copy_progress(base, pid, start, amt)
    time.sleep(1)

print("所有檔案與隱藏目錄複製完成。")
