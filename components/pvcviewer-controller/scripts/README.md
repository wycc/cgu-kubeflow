# PVCViewer 公開分享資源檢查工具

## 概述

`check-public-share-resources.sh` 是一個用於驗證現有 PVCViewer 是否正確配置公開分享資源的診斷工具。

## 功能

此腳本會檢查每個配置了 `networking` 的 PVCViewer，並驗證以下資源：

1. ✅ **Finalizer** - `pvcviewer.kubeflow.org/public-share-cleanup`
2. ✅ **VirtualService (share)** - `pvcviewer-share-{name}`
3. ✅ **EnvoyFilter** - `bypass-auth-pvcviewer-share-{namespace}-{name}`
4. ✅ **AuthorizationPolicy (Gateway)** - `allow-pvcviewer-share-{namespace}-{name}-gw`
5. ✅ **AuthorizationPolicy (Backend)** - `allow-pvcviewer-share-{name}-inbound`

## 使用方法

### 基本使用

```bash
./scripts/check-public-share-resources.sh
```

### 前置要求

1. **kubectl** 已安裝並配置
2. **jq** 已安裝（用於 JSON 解析）
3. 具有讀取以下資源的權限：
   - PVCViewer (所有 namespace)
   - VirtualService
   - EnvoyFilter (istio-system)
   - AuthorizationPolicy (所有 namespace)

### 安裝 jq（如果尚未安裝）

```bash
# Ubuntu/Debian
sudo apt-get install jq

# macOS
brew install jq

# RHEL/CentOS
sudo yum install jq
```

## 輸出說明

### 正常輸出範例

```
========================================
PVCViewer 公開分享資源檢查工具
========================================

正在掃描所有 namespace 的 PVCViewer...

Namespace: kubeflow-user-example-com
----------------------------------------

檢查 PVCViewer: my-pvcviewer
  ✓ 已配置 Networking

  1. 檢查 Finalizer
     ✓ Finalizer 存在

  2. 檢查 Share VirtualService
     ✓ VirtualService 存在: pvcviewer-share-my-pvcviewer
     ✓ 路徑配置正確: /pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/

  3. 檢查 EnvoyFilter
     ✓ EnvoyFilter 存在: bypass-auth-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer
     ✓ 追蹤標籤正確

  4. 檢查 Gateway AuthorizationPolicy
     ✓ AuthorizationPolicy (Gateway) 存在: allow-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer-gw
     ✓ 路徑配置正確: /pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/*

  5. 檢查 Backend AuthorizationPolicy
     ✓ AuthorizationPolicy (Backend) 存在: allow-pvcviewer-share-my-pvcviewer-inbound
     ✓ HTTP 方法限制正確: GET,HEAD,OPTIONS
     ✓ 路徑配置數量正確（2個）

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ✓ 所有公開分享資源配置正確
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

========================================
檢查總結
========================================
總 PVCViewer 數量:            3
配置 Networking 的數量:       2
公開分享資源完整的數量:       2
公開分享資源缺失的數量:       0

✓ 所有 PVCViewer 的公開分享資源都配置正確！
```

### 發現問題時的輸出

```
檢查 PVCViewer: broken-pvcviewer
  ✓ 已配置 Networking

  1. 檢查 Finalizer
     ✗ Finalizer 缺失

  2. 檢查 Share VirtualService
     ✗ VirtualService 不存在: pvcviewer-share-broken-pvcviewer

  3. 檢查 EnvoyFilter
     ✗ EnvoyFilter 不存在: bypass-auth-pvcviewer-share-ns-broken-pvcviewer

  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ✗ 發現缺失或異常的資源
  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

建議操作:
1. 對於缺失資源的 PVCViewer，觸發重新協調：
   kubectl annotate pvcviewer <name> -n <namespace> reconcile=$(date +%s)

2. 檢查 Controller 日誌：
   kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager --tail=100

3. 如果問題持續，檢查 RBAC 權限：
   kubectl get clusterrole pvcviewer-controller-role -o yaml | grep -A 5 'envoyfilters\|authorizationpolicies'
```

## 退出碼

- **0**: 所有 PVCViewer 的公開分享資源都配置正確
- **1**: 發現至少一個 PVCViewer 缺失公開分享資源

## 檢查項目詳解

### 1. Finalizer 檢查
驗證 PVCViewer 是否包含 `pvcviewer.kubeflow.org/public-share-cleanup` finalizer，用於確保刪除時清理 istio-system 資源。

### 2. VirtualService (share) 檢查
- **名稱格式**: `pvcviewer-share-{name}`
- **路徑驗證**: 確認路徑為 `{basePrefix}/{namespace}/{name}/share/`

### 3. EnvoyFilter 檢查
- **名稱格式**: `bypass-auth-pvcviewer-share-{namespace}-{name}`
- **Namespace**: `istio-system`
- **標籤驗證**: 確認包含追蹤標籤

### 4. AuthorizationPolicy (Gateway) 檢查
- **名稱格式**: `allow-pvcviewer-share-{namespace}-{name}-gw`
- **Namespace**: `istio-system`
- **路徑驗證**: 確認路徑為 `{basePrefix}/{namespace}/{name}/share/*`

### 5. AuthorizationPolicy (Backend) 檢查
- **名稱格式**: `allow-pvcviewer-share-{name}-inbound`
- **方法驗證**: 確認只允許 GET、HEAD、OPTIONS
- **路徑驗證**: 確認包含 2 個路徑（外部和內部）

## 故障排除

### 問題：腳本報錯 "jq: command not found"

**解決方法**: 安裝 jq
```bash
sudo apt-get install jq  # Ubuntu/Debian
brew install jq          # macOS
```

### 問題：腳本報錯權限不足

**解決方法**: 確保當前 kubectl 上下文有足夠權限
```bash
# 檢查當前上下文
kubectl config current-context

# 檢查權限
kubectl auth can-i list pvcviewer --all-namespaces
kubectl auth can-i list virtualservice --all-namespaces
kubectl auth can-i list envoyfilter -n istio-system
kubectl auth can-i list authorizationpolicy --all-namespaces
```

### 問題：發現缺失的資源

**修復步驟**:

1. **觸發重新協調**（推薦）:
```bash
kubectl annotate pvcviewer <name> -n <namespace> reconcile=$(date +%s)
```

2. **檢查 Controller 日誌**:
```bash
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager --tail=100 | grep "public share"
```

3. **手動刪除並重建 PVCViewer**（如果上述方法無效）:
```bash
# 備份配置
kubectl get pvcviewer <name> -n <namespace> -o yaml > pvcviewer-backup.yaml

# 刪除
kubectl delete pvcviewer <name> -n <namespace>

# 重建
kubectl apply -f pvcviewer-backup.yaml
```

## 自動化使用

### 在 CI/CD 中使用

```bash
#!/bin/bash
set -e

# 運行檢查
./scripts/check-public-share-resources.sh

# 腳本會返回非零退出碼如果發現問題
if [ $? -ne 0 ]; then
    echo "Public share resources check failed!"
    exit 1
fi

echo "All checks passed!"
```

### 定期檢查（Cron）

```bash
# 每小時檢查一次
0 * * * * /path/to/scripts/check-public-share-resources.sh >> /var/log/pvcviewer-check.log 2>&1
```

### Kubernetes CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: pvcviewer-share-check
  namespace: kubeflow
spec:
  schedule: "0 * * * *"  # 每小時
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: pvcviewer-checker
          containers:
          - name: checker
            image: bitnami/kubectl:latest
            command:
            - /bin/bash
            - -c
            - |
              apt-get update && apt-get install -y jq
              /scripts/check-public-share-resources.sh
            volumeMounts:
            - name: scripts
              mountPath: /scripts
          volumes:
          - name: scripts
            configMap:
              name: pvcviewer-check-script
              defaultMode: 0755
          restartPolicy: OnFailure
```

## 相關文檔

- [公開分享功能說明](../PUBLIC_SHARE_FEATURE.md)
- [部署指南](../DEPLOYMENT.md)
- [設計文檔](../PUBLIC_SHARE_DESIGN.md)