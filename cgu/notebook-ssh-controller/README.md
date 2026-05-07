# Notebook SSH Controller

一個 Kubernetes Operator，用於自動為 Kubeflow Notebook 建立 SSH 存取服務，並配置 Istio 流量規則，讓使用者能透過 SSH 直接連線至 Notebook Pod。

透過在 Notebook 上設定 **Label**（`cgu.kubeflow.org/sshservice: "true"`）即可啟用或停用 SSH Service，方便在 Kubeflow UI 中以標籤控制，無需手動建立任何 Kubernetes 資源。

---

## 目錄

- [專案概述](#專案概述)
- [架構說明](#架構說明)
- [運作原理](#運作原理)
  - [NotebookReconciler（SSH Service 管理）](#notebookreconcilerssh-service-管理)
  - [StatefulSetReconciler（Istio 流量排除）](#statefulsetreconcileristio-流量排除)
- [前置需求](#前置需求)
- [快速開始](#快速開始)
  - [建構 Docker 映像檔](#建構-docker-映像檔)
  - [部署到 Kubernetes 叢集](#部署到-kubernetes-叢集)
  - [移除部署](#移除部署)
- [設定說明](#設定說明)
  - [啟用 SSH Service](#啟用-ssh-service)
  - [讀取指派的 NodePort](#讀取指派的-nodeport)
  - [停用 SSH Service](#停用-ssh-service)
  - [Controller 啟動參數](#controller-啟動參數)
- [常數與 API 參照](#常數與-api-參照)
- [RBAC 權限](#rbac-權限)
- [目錄結構](#目錄結構)
- [技術細節](#技術細節)
- [開發指南](#開發指南)

---

## 專案概述

Kubeflow Notebook 預設只對外暴露 HTTP 服務（如 JupyterLab），不提供 SSH 存取。此 Controller 以 **Label 驅動** 的方式，自動完成以下工作：

1. **建立 NodePort Service**：讓外部使用者可透過 SSH 連線到 Notebook Pod（容器 Port 22，Service Port 2222）。
2. **回寫 NodePort 至 Notebook Annotation**：讓 Kubeflow UI 可直接讀取並顯示給使用者，無需手動查詢。
3. **設定 Istio 排除規則**：在 StatefulSet 的 Pod Template 上加入 Annotation，讓 Istio Sidecar（Envoy Proxy）略過對 SSH 流量的攔截，確保 SSH 連線正常運作。

---

## 架構說明

```
┌──────────────────────────────────────────────────────────────┐
│                     Kubernetes Cluster                         │
│                                                                │
│  ┌────────────────────────────────────────────────────────┐  │
│  │            notebook-ssh-controller (Operator)            │  │
│  │                                                          │  │
│  │  ┌────────────────────────┐  ┌──────────────────────┐  │  │
│  │  │   NotebookReconciler   │  │StatefulSetReconciler  │  │  │
│  │  │                        │  │                       │  │  │
│  │  │ 監聽 Notebook CRD      │  │ 監聽 StatefulSet      │  │  │
│  │  │ 依 Label 建立/刪除     │  │ 查父 Notebook Label   │  │  │
│  │  │ SSH NodePort Service   │  │ 管理 Istio 排除規則   │  │  │
│  │  │ 回寫 NodePort Annotation│  │                      │  │  │
│  │  └────────────┬───────────┘  └──────────┬────────────┘  │  │
│  └───────────────┼────────────────────────--┼───────────────┘  │
│                  │                           │                   │
│     ┌────────────▼──────────┐   ┌───────────▼──────────────┐  │
│     │  Notebook (CRD)        │   │  StatefulSet             │  │
│     │  Labels:               │   │  Pod Template Annotation:│  │
│     │   cgu.kubeflow.org/    │   │  excludeInboundPorts=22  │  │
│     │   sshservice: "true"   │   └──────────────────────────┘  │
│     │  Annotations:          │                                   │
│     │   cgu.kubeflow.org/    │                                   │
│     │   ssh-nodeport:"31234" │                                   │
│     └────────────┬───────────┘                                   │
│                  │                                                │
│     ┌────────────▼──────────────┐                               │
│     │  SSH Service (NodePort)    │                               │
│     │  <notebook>-ssh-service    │                               │
│     │  Port: 2222 → 22           │                               │
│     └────────────────────────────┘                               │
└──────────────────────────────────────────────────────────────────┘
```

---

## 運作原理

### NotebookReconciler（SSH Service 管理）

檔案：[`controllers/notebook_controller.go`](controllers/notebook_controller.go)

監聽 `kubeflow.org/v1beta1` 的 `Notebook` 資源，依 Label 執行以下協調邏輯：

| Notebook Label 狀態 | Service 存在？ | 行為 |
|---------------------|---------------|------|
| `sshservice: "true"` | 否 | 建立 NodePort SSH Service，Requeue 等待 NodePort 指派 |
| `sshservice: "true"` | 是 | 讀取 NodePort，回寫至 Notebook Annotation |
| 未設定或非 `"true"` | 是 | 刪除 Service，清除 Notebook Annotation |
| 未設定或非 `"true"` | 否 | 無操作（已達期望狀態） |
| Notebook 正在刪除 | 任意 | 由 OwnerReference 機制自動清理 Service |

**建立的 Service 固定規格：**

| 欄位 | 值 |
|------|----|
| 名稱 | `<notebook-name>-ssh-service` |
| 類型 | `NodePort` |
| Service Port | `2222` |
| 目標 Container Port | `22` |
| Protocol | `TCP` |
| Selector | `notebook-name=<notebook-name>` |
| OwnerReference | 指向對應 Notebook |

**回寫的 Annotation：**

| Annotation Key | 範例值 | 說明 |
|----------------|--------|------|
| `cgu.kubeflow.org/ssh-nodeport` | `"31234"` | Kubernetes 指派的 NodePort，供 UI 顯示 |

### StatefulSetReconciler（Istio 流量排除）

檔案：[`controllers/statefulset_controller.go`](controllers/statefulset_controller.go)

監聽所有 `StatefulSet`，針對 Kubeflow Notebook 建立的 StatefulSet，透過 **OwnerReference 查詢父 Notebook** 是否啟用 SSH。

**判斷 Notebook StatefulSet 的條件（符合任一）：**
- Label 含有 `notebook-name`
- Label `app.kubernetes.io/component=notebook`
- Label `kubeflow-resource-type=notebook`
- OwnerReference 的 Kind 為 `Notebook`（APIVersion `kubeflow.org/v1beta1`）

**處理流程：**

| 父 Notebook Label | Istio Annotation 存在？ | 行為 |
|-------------------|------------------------|------|
| `sshservice: "true"` | 否 | 新增排除 Annotation |
| `sshservice: "true"` | 是 | 無操作 |
| 未啟用 | 是 | 移除排除 Annotation |
| 未啟用 | 否 | 無操作 |

加入的 Annotation（位於 `StatefulSet.spec.template.metadata.annotations`）：

```yaml
traffic.sidecar.istio.io/excludeInboundPorts: "22"
```

---

## 前置需求

| 軟體 | 最低版本 |
|------|----------|
| Go | 1.23 |
| Kubernetes | 1.23+ |
| Kubeflow（含 Notebook Controller） | 需已安裝 |
| Istio（選用） | 若使用 Istio 服務網格則需要 |
| Docker | 任意版本（用於建構映像檔） |
| kubectl | 1.23+ |

---

## 快速開始

### 建構 Docker 映像檔

```bash
# 建構映像檔（預設版本 1.1.0）
make docker-build

# 指定版本號
make docker-build VER=1.2.0

# 推送映像檔到 Registry
make docker-push

# 推送指定版本
make docker-push VER=1.2.0
```

映像檔標籤格式：`cguaicadmin/notebook-ssh-controller:<VERSION>`

### 部署到 Kubernetes 叢集

Controller 將部署到 `cgu` Namespace。

```bash
# 確認 Namespace 存在
kubectl create namespace cgu --dry-run=client -o yaml | kubectl apply -f -

# 套用 RBAC 設定
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

# 部署 Controller
make deploy
```

確認部署狀態：

```bash
kubectl get deployment -n cgu notebook-ssh-controller-manager
kubectl get pods -n cgu -l control-plane=controller-manager
```

### 移除部署

```bash
make undeploy
```

---

## 設定說明

### 啟用 SSH Service

在 Notebook 資源的 `metadata.labels` 加入以下 Label，Controller 就會自動建立 SSH NodePort Service：

```yaml
apiVersion: kubeflow.org/v1beta1
kind: Notebook
metadata:
  name: my-notebook
  namespace: user-namespace
  labels:
    cgu.kubeflow.org/sshservice: "true"   # ← 加上這個 Label 即可啟用
spec:
  template:
    spec:
      containers:
        - name: my-notebook
          image: your-notebook-image-with-sshd:latest
          # 不需要宣告第二個 Port，Controller 固定使用 Port 22
```

> **注意**：Notebook 的容器映像檔必須已安裝並啟動 SSH Daemon（`sshd`），並監聽 Port **22**。

Controller 會自動建立以下 Service（無需手動建立）：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-notebook-ssh-service
  namespace: user-namespace
  ownerReferences:
    - apiVersion: kubeflow.org/v1beta1
      kind: Notebook
      name: my-notebook
spec:
  type: NodePort
  selector:
    notebook-name: my-notebook
  ports:
    - name: tcp-ssh
      protocol: TCP
      port: 2222
      targetPort: 22
      nodePort: <系統自動指派>
```

### 讀取指派的 NodePort

Service 建立後，Controller 會自動將 NodePort 回寫至 Notebook 的 Annotation：

```bash
# 查詢已指派的 NodePort
kubectl get notebook my-notebook -n user-namespace \
  -o jsonpath='{.metadata.annotations.cgu\.kubeflow\.org/ssh-nodeport}'
```

使用者透過 SSH 連線：

```bash
# 取得任一 Node 的 IP
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')

# 取得 NodePort
NODE_PORT=$(kubectl get notebook my-notebook -n user-namespace \
  -o jsonpath='{.metadata.annotations.cgu\.kubeflow\.org/ssh-nodeport}')

# 連線
ssh -p ${NODE_PORT} user@${NODE_IP}
```

### 停用 SSH Service

移除 Label 或將值改為非 `"true"` 即可停用：

```bash
# 方法一：移除 Label
kubectl label notebook my-notebook -n user-namespace cgu.kubeflow.org/sshservice-

# 方法二：設為 false
kubectl label notebook my-notebook -n user-namespace cgu.kubeflow.org/sshservice=false --overwrite
```

Controller 會自動刪除 SSH Service，並清除 `cgu.kubeflow.org/ssh-nodeport` Annotation。

### Controller 啟動參數

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `--metrics-bind-address` | `127.0.0.1:8080` | Prometheus Metrics 端點位址 |
| `--health-probe-bind-address` | `:8081` | 健康檢查端點位址 |
| `--leader-elect` | `false` | 啟用 Leader Election（多副本高可用時使用） |

健康檢查端點：
- **存活探針（Liveness）**：`GET /healthz`
- **就緒探針（Readiness）**：`GET /readyz`

---

## 常數與 API 參照

定義於 [`controllers/constants.go`](controllers/constants.go)：

| 常數名稱 | 值 | 說明 |
|----------|----|------|
| `SSHServiceLabel` | `cgu.kubeflow.org/sshservice` | 啟用 SSH Service 的 Label Key |
| `SSHNodePortAnnotation` | `cgu.kubeflow.org/ssh-nodeport` | 回寫 NodePort 的 Annotation Key |
| `IstioExcludePortAnnotation` | `traffic.sidecar.istio.io/excludeInboundPorts` | Istio 流量排除 Annotation Key |
| `SSHContainerPort` | `22` | 容器內 SSH Daemon 監聽的 Port |
| `SSHServicePort` | `2222` | Service 對外的 Port |
| `IstioExcludePortValue` | `"22"` | 排除的 Istio 攔截 Port 字串 |

---

## RBAC 權限

Controller 需要以下 Kubernetes RBAC 權限（定義於 [`config/rbac/role.yaml`](config/rbac/role.yaml)）：

| 資源群組 | 資源 | 權限 |
|----------|------|------|
| `kubeflow.org` | `notebooks` | get, list, watch, update, patch |
| `kubeflow.org` | `notebooks/status` | get, update, patch |
| `kubeflow.org` | `notebooks/finalizers` | update |
| `""` (core) | `services` | get, list, watch, create, update, patch, delete |
| `apps` | `statefulsets` | get, list, watch, update, patch |
| `apps` | `statefulsets/status` | get, update, patch |
| `""` (core) | `configmaps` | get, list, watch, create, update, patch, delete |
| `coordination.k8s.io` | `leases` | get, list, watch, create, update, patch, delete |
| `""` (core) | `events` | create, patch |

---

## 目錄結構

```
notebook-ssh-controller/
├── main.go                          # 程式進入點，初始化並啟動 Controller Manager
├── go.mod                           # Go 模組定義
├── go.sum                           # Go 依賴鎖定檔
├── Dockerfile                       # 多階段建構的 Docker 映像檔定義
├── Makefile                         # 常用指令集（建構、部署、測試）
├── OperatorPattern.md               # Kubernetes Operator 模式說明文件
├── README.md                        # 本文件
├── controllers/
│   ├── constants.go                 # 共用常數（Label、Annotation、Port 定義）
│   ├── notebook_controller.go       # NotebookReconciler：依 Label 管理 SSH Service 與回寫 NodePort
│   └── statefulset_controller.go   # StatefulSetReconciler：依父 Notebook Label 管理 Istio 排除規則
└── config/
    ├── manager/
    │   ├── manager.yaml             # Controller Deployment 設定
    │   └── kustomization.yaml       # Kustomize 設定
    └── rbac/
        ├── role.yaml                # ClusterRole 權限定義
        └── role_binding.yaml        # ClusterRoleBinding 設定
```

---

## 技術細節

### 技術棧

| 技術 | 版本 | 用途 |
|------|------|------|
| Go | 1.23 | 主要開發語言 |
| [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) | v0.11.2 | Kubernetes Controller 框架 |
| [kubeflow/notebook-controller](https://github.com/kubeflow/kubeflow) | v1beta1 | Notebook CRD API |
| k8s.io/api | v0.23.5 | Kubernetes API 型別定義 |
| k8s.io/client-go | v0.23.5 | Kubernetes 客戶端 |

### 映像檔說明

使用多階段建構（Multi-stage Build）：

1. **Builder Stage**（`golang:1.23`）：編譯 Go 程式，產生靜態連結的二進位檔案 `manager`
2. **Runtime Stage**（`gcr.io/distroless/static:nonroot`）：最小化 distroless 映像檔，以非 root 使用者（UID 65532）執行，提升安全性

### 設計決策

- **Label 驅動**：使用 Notebook Label 決定是否啟用 SSH，讓 Kubeflow UI 只需操作 Label 即可控制 SSH Service 的生命週期。
- **固定 Port**：容器 Port 固定為 22，Service Port 固定為 2222，所有使用同一映像檔的 Notebook 行為一致，無需額外設定。
- **NodePort 回寫**：Service 建立後自動將系統指派的 NodePort 回寫至 Notebook Annotation（`cgu.kubeflow.org/ssh-nodeport`），避免使用者手動查詢。回寫時設有值比較，相同值不觸發 Update，防止協調循環無限重複。
- **OwnerReference 清理**：SSH Service 設定 Notebook 為擁有者，刪除 Notebook 時 Service 自動被 Kubernetes GC 回收，無需額外清理邏輯。
- **Istio 整合**：透過在 StatefulSet Pod Template 加入 `traffic.sidecar.istio.io/excludeInboundPorts: "22"` Annotation，確保 Istio Sidecar 不干擾 SSH 流量的直通連線。

### Operator 模式

本專案遵循 Kubernetes Operator Pattern 實作，詳細說明請參考 [`OperatorPattern.md`](OperatorPattern.md)。

---

## 開發指南

### 本地執行

```bash
# 安裝依賴
go mod download

# 確保 kubeconfig 已設定（指向目標叢集）
export KUBECONFIG=~/.kube/config

# 直接執行（不打包 Docker）
go run main.go --metrics-bind-address=":8080" --health-probe-bind-address=":8081"
```

### 執行測試

```bash
make test

# 查看覆蓋率報告
go tool cover -html=cover.out
```

### 清理建構產出

```bash
make clean
```

### 更新版本

編輯 [`Makefile`](Makefile) 中的 `VER` 變數，並同步更新 [`config/manager/manager.yaml`](config/manager/manager.yaml) 中的 `image` 欄位：

```makefile
VER ?= 1.2.0  # 更新此處
```

```yaml
# config/manager/manager.yaml
image: cguaicadmin/notebook-ssh-controller:1.2.0  # 同步更新此處
```
