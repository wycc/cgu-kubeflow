# PVCViewer Controller 設計文件

## 概述

PVCViewer 是一個 Kubernetes Operator，旨在為 Kubeflow 生態系統提供一個簡單的方式來檢視和管理 PersistentVolumeClaim (PVC) 中的數據。透過創建 PVCViewer 自定義資源（CR），使用者可以快速啟動一個檔案瀏覽器來檢查、下載、上傳和操作儲存在 PVC 中的數據。

### 主要特性

- **自動化資源管理**：自動創建和管理 Deployment、Service 和 VirtualService
- **靈活的 PodSpec 配置**：支援自定義或使用預設的檔案瀏覽器配置
- **RWO 卷智能調度**：自動處理 ReadWriteOnce 卷的調度約束
- **Istio 整合**：透過 VirtualService 提供安全的 HTTP 路由
- **可擴展性**：設計允許用於其他使用案例（如 TensorBoard、Notebook 等）

## 架構設計

### 系統架構圖

```
┌─────────────────────────────────────────────────────────────┐
│                      Kubeflow Platform                       │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────┐         ┌────────────────────┐        │
│  │  Volumes Web App │────────▶│  PVCViewer CR      │        │
│  │  (or other UI)   │         │  (Custom Resource) │        │
│  └──────────────────┘         └─────────┬──────────┘        │
│                                          │                    │
│                                          ▼                    │
│  ┌───────────────────────────────────────────────────────┐  │
│  │         PVCViewer Controller (Reconcile Loop)         │  │
│  └───┬───────────────┬───────────────┬───────────────┬───┘  │
│      │               │               │               │       │
│      ▼               ▼               ▼               ▼       │
│  ┌─────────┐   ┌─────────┐   ┌──────────────┐  ┌────────┐ │
│  │Deployment│   │ Service │   │VirtualService│  │ Status │ │
│  │         │   │         │   │   (Istio)    │  │ Update │ │
│  └────┬────┘   └────┬────┘   └──────┬───────┘  └────────┘ │
│       │             │               │                       │
│       ▼             ▼               ▼                       │
│  ┌─────────────────────────────────────────────┐           │
│  │           Running Pod (FileBrowser)          │           │
│  │                                               │           │
│  │  ┌────────────────────────────────────────┐ │           │
│  │  │  PVC Mount (/data)                     │ │           │
│  │  └────────────────────────────────────────┘ │           │
│  └─────────────────────────────────────────────┘           │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

### 核心組件

#### 1. Custom Resource Definition (CRD)

**檔案位置**：[`api/v1alpha1/pvcviewer_types.go`](api/v1alpha1/pvcviewer_types.go)

定義了 `PVCViewer` 自定義資源的結構：

```go
type PVCViewerSpec struct {
    PVC           string              // 要檢視的 PVC 名稱
    PodSpec       corev1.PodSpec      // 自定義 Pod 規格
    Networking    Networking          // 網路設定
    RWOScheduling bool                // RWO 卷智能調度開關
}

type Networking struct {
    TargetPort intstr.IntOrString    // 應用程式目標端口
    BasePrefix string                // VirtualService 的基礎路徑前綴
    Rewrite    string                // URL 重寫規則
    Timeout    string                // 請求超時設定
}

type PVCViewerStatus struct {
    Conditions []appsv1.DeploymentCondition  // Deployment 狀態條件
    Ready      bool                           // 是否就緒
    URL        *string                        // VirtualService URL
}
```

#### 2. Controller (Reconciler)

**檔案位置**：[`controllers/pvcviewer_controller.go`](controllers/pvcviewer_controller.go)

負責實現主要的業務邏輯，遵循 Kubernetes Operator Pattern。

**核心方法**：

- [`Reconcile()`](controllers/pvcviewer_controller.go:96-146)：主要的協調循環
- [`reconcileDeployment()`](controllers/pvcviewer_controller.go:149-207)：管理 Deployment 資源
- [`reconcileService()`](controllers/pvcviewer_controller.go:210-250)：管理 Service 資源
- [`reconcileVirtualService()`](controllers/pvcviewer_controller.go:252-335)：管理 VirtualService 資源
- [`reconcileStatus()`](controllers/pvcviewer_controller.go:338-368)：更新狀態
- [`generateAffinity()`](controllers/pvcviewer_controller.go:372-445)：生成 Node Affinity

#### 3. Webhook

**檔案位置**：[`api/v1alpha1/pvcviewer_webhook.go`](api/v1alpha1/pvcviewer_webhook.go)

實現了兩個 webhook：

- **Mutating Webhook**：[`Default()`](api/v1alpha1/pvcviewer_webhook.go:69-146) - 為空的 PodSpec 設定預設值
- **Validating Webhook**：[`validate()`](api/v1alpha1/pvcviewer_webhook.go:152-176) - 驗證 CR 的有效性

## 詳細設計

### 1. Reconciliation 循環

Controller 的協調循環實現了以下邏輯：

```
Reconcile 開始
    │
    ├─▶ 取得 PVCViewer CR
    │   └─▶ 如果不存在 → 返回（資源已刪除）
    │
    ├─▶ 檢查刪除時間戳
    │   └─▶ 如果正在刪除 → 更新狀態並返回
    │
    ├─▶ 生成通用標籤
    │   └─▶ app.kubernetes.io/name
    │   └─▶ app.kubernetes.io/instance
    │   └─▶ app.kubernetes.io/part-of
    │
    ├─▶ 協調 Deployment
    │   ├─▶ 檢查是否存在
    │   ├─▶ 如果啟用 RWOScheduling 且是新建
    │   │   └─▶ 生成 Node Affinity
    │   └─▶ 創建或更新 Deployment
    │
    ├─▶ 協調 Service
    │   └─▶ 如果定義了 Networking → 創建或更新
    │
    ├─▶ 協調 VirtualService
    │   └─▶ 如果定義了 Networking → 創建或更新
    │
    └─▶ 協調 Status
        └─▶ 更新 Ready 狀態和 URL
```

### 2. RWO 卷智能調度

**問題**：ReadWriteOnce (RWO) 卷一次只能掛載到一個節點上。如果 PVC 已經被其他 Pod 使用，新的 Viewer Pod 可能無法啟動。

**解決方案**：

當 [`RWOScheduling`](api/v1alpha1/pvcviewer_types.go:47) 設為 `true` 時，Controller 會：

1. **檢查 PVC 存取模式**：確認是否為 RWO
2. **查找使用該 PVC 的 Pod**：掃描同一命名空間的所有 Pod
3. **獲取節點名稱**：找出 PVC 當前掛載的節點
4. **生成 Node Affinity**：創建 PreferredDuringSchedulingIgnoredDuringExecution 規則

**實現細節**（[`generateAffinity()`](controllers/pvcviewer_controller.go:372-445)）：

```go
affinity := &corev1.Affinity{
    NodeAffinity: &corev1.NodeAffinity{
        PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
            {
                Weight: 100,
                Preference: corev1.NodeSelectorTerm{
                    MatchExpressions: []corev1.NodeSelectorRequirement{
                        {
                            Key:      "kubernetes.io/hostname",
                            Operator: "In",
                            Values:   []string{nodeName},
                        },
                    },
                },
            },
        },
    },
}
```

**注意事項**：

- 使用 `PreferredDuringScheduling` 而非 `RequiredDuringScheduling`，提供更高的靈活性
- 只在創建新 Deployment 時設定 Affinity
- 如果 PVC 在多個節點上使用，會跳過 Affinity 設定
- 使用 `RecreateDeploymentStrategyType` 確保 Pod 重啟時應用新的 Affinity

### 3. Webhook 預設值處理

**預設 PodSpec 載入順序**：

1. **檢查環境變數**：`DEFAULT_POD_SPEC_PATH`
2. **從檔案載入**：如果設定了路徑，從 YAML/JSON 檔案載入
3. **使用硬編碼預設值**：FileBrowser 配置

**預設 FileBrowser 配置**（[`Default()`](api/v1alpha1/pvcviewer_webhook.go:91-131)）：

```go
Container {
    Name:  "pvcviewer",
    Image: "filebrowser/filebrowser:latest",
    Env: []corev1.EnvVar{
        {Name: "FB_ADDRESS", Value: "0.0.0.0"},
        {Name: "FB_PORT", Value: "8080"},
        {Name: "FB_DATABASE", Value: "/tmp/filebrowser.db"},
        {Name: "FB_NOAUTH", Value: "true"},
        {Name: "FB_BASEURL", Value: "{basePrefix}/{namespace}/{name}/"},
    },
    VolumeMounts: []corev1.VolumeMount{
        {Name: "viewer-volume", MountPath: "/data"},
    },
}
```

**自動 Volume 注入**：

無論使用哪種預設值來源，Webhook 都會自動添加：

```go
Volume {
    Name: "viewer-volume",
    VolumeSource: corev1.VolumeSource{
        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
            ClaimName: r.Spec.PVC,
        },
    },
}
```

### 4. 網路配置

#### Service 配置

- **類型**：ClusterIP
- **端口**：80（固定）
- **目標端口**：來自 [`Networking.TargetPort`](api/v1alpha1/pvcviewer_types.go:53)
- **選擇器**：使用通用標籤匹配 Pod

#### VirtualService 配置

**URL 結構**：

```
{BasePrefix}/{Namespace}/{Name}/
```

例如：`/pvcviewer/kubeflow-user-example-com/pvcviewer-sample/`

**路由規則**（[`reconcileVirtualService()`](controllers/pvcviewer_controller.go:293-323)）：

```yaml
spec:
  hosts: ["*"]
  gateways: ["kubeflow/kubeflow-gateway"]  # 可透過環境變數自定義
  http:
    - match:
        - uri:
            prefix: "/pvcviewer/{namespace}/{name}/"
      rewrite:
        uri: "/"  # 或自定義 rewrite
      route:
        - destination:
            host: "pvcviewer-{name}.{namespace}.svc.cluster.local"
            port:
              number: 80
      timeout: "{custom-timeout}"  # 可選
```

**Istio Gateway 配置**：

- 預設：`kubeflow/kubeflow-gateway`
- 可透過環境變數 `ISTIO_GATEWAY` 自定義

### 5. 資源命名和標籤策略

**資源命名**：

所有生成的資源使用前綴 `pvcviewer-` 加上 PVCViewer CR 的名稱：

- Deployment: `pvcviewer-{name}`
- Service: `pvcviewer-{name}`
- VirtualService: `pvcviewer-{name}`

**標籤策略**：

```go
commonLabels := map[string]string{
    "app.kubernetes.io/name":     "{viewer.Name}",
    "app.kubernetes.io/instance": "pvcviewer-{viewer.Name}",
    "app.kubernetes.io/part-of":  "pvc-viewer",
}
```

這些標籤用於：
- Pod 選擇器
- Service 選擇器
- 資源識別和管理
- 過濾控制器創建的資源（在 RWO 調度中）

### 6. 狀態管理

**Status 更新**（[`reconcileStatus()`](controllers/pvcviewer_controller.go:338-368)）：

```go
status := PVCViewerStatus{
    Ready: Replicas == ReadyReplicas,
    URL:   "{basePrefix}/{namespace}/{name}/",
    Conditions: []DeploymentCondition{...},
}
```

**狀態條件**：

- 從底層 Deployment 的條件鏡像而來
- 只追加最新的條件，避免重複

**就緒檢查**：

```go
Ready = (deployment.Spec.Replicas == deployment.Status.ReadyReplicas)
```

## 配置管理

### 環境變數

| 變數名稱 | 用途 | 預設值 |
|---------|------|--------|
| `DEFAULT_POD_SPEC_PATH` | 預設 PodSpec 配置檔案路徑 | 無 |
| `ISTIO_GATEWAY` | Istio Gateway 名稱 | `kubeflow/kubeflow-gateway` |

### 命令列參數（[`main.go`](main.go:53-66)）

| 參數 | 預設值 | 說明 |
|------|--------|------|
| `--metrics-bind-address` | `:8080` | Metrics 端點綁定地址 |
| `--health-probe-bind-address` | `:8081` | Health probe 綁定地址 |
| `--leader-elect` | `false` | 啟用 Leader Election |

### 自定義預設 PodSpec

**步驟**：

1. 創建 YAML 檔案定義 PodSpec：
   ```yaml
   containers:
   - name: custom-viewer
     image: my-custom-image:latest
     # ...其他配置
   ```

2. 掛載到 Controller：
   ```yaml
   volumes:
   - name: config
     configMap:
       name: pvcviewer-defaults
   ```

3. 設定環境變數：
   ```yaml
   env:
   - name: DEFAULT_POD_SPEC_PATH
     value: /config/podspec.yaml
   ```

## RBAC 權限

### Controller 權限（[`pvcviewer_controller.go`](controllers/pvcviewer_controller.go:69-81)）

```yaml
# PVCViewer 資源
- apiGroups: ["kubeflow.org"]
  resources: ["pvcviewers", "pvcviewers/status", "pvcviewers/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# 子資源
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update"]

- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch", "create", "update"]

- apiGroups: ["networking.istio.io"]
  resources: ["virtualservices"]
  verbs: ["get", "list", "watch", "create", "update"]

# 唯讀資源
- apiGroups: [""]
  resources: ["pods", "persistentvolumeclaims"]
  verbs: ["get", "list", "watch"]
```

## 部署架構

### 依賴項

1. **Kubernetes** (v1.19+)
2. **Istio** - 用於 VirtualService
3. **Cert-Manager** - 用於 Webhook 證書管理

### 部署流程

```bash
# 1. 安裝 CRD 和 RBAC
kubectl apply -k config/default

# 2. 構建並推送映像檔
docker build -t kubeflow-pvc-viewer:v1.0.0 .
docker push kubeflow-pvc-viewer:v1.0.0

# 3. 更新部署
kubectl -n kubeflow set image deployment/pvc-viewer-controller-manager \
  manager=kubeflow-pvc-viewer:v1.0.0

# 4. 創建範例
kubectl apply -k config/samples
```

## 安全考量

### 1. 認證和授權

- **預設配置**：FileBrowser 使用 `FB_NOAUTH=true`（無認證）
- **建議**：在生產環境中應啟用認證或依賴 Istio 的 AuthorizationPolicy

### 2. 網路隔離

- 使用 Istio VirtualService 控制訪問
- 建議配合 NetworkPolicy 限制 Pod 間通訊

### 3. PVC 訪問權限

- PVCViewer 需要能夠列出和讀取 PVC
- 應限制在特定命名空間內操作

### 4. Webhook 安全

- 使用 Cert-Manager 自動管理 TLS 證書
- Webhook 使用 `failurePolicy: fail` 確保安全

## 錯誤處理和恢復

### 常見錯誤場景

1. **PVC 不存在**
   - 允許 Deployment 創建失敗
   - 錯誤會在 Deployment 狀態中顯示

2. **RWO 卷衝突**
   - 使用 Affinity 引導調度
   - 使用 Recreate 策略避免多副本

3. **Webhook 配置載入失敗**
   - 記錄錯誤但不阻止處理
   - 由 Validating Webhook 捕獲

4. **VirtualService 創建失敗**
   - 記錄錯誤並重試
   - Status 會反映失敗狀態

### 重試機制

- Controller Runtime 提供自動重試
- 錯誤會導致重新排隊協調

## 監控和可觀測性

### Metrics

Controller 暴露 Prometheus metrics 在 `:8080/metrics`：

- Controller Runtime 標準 metrics
- Reconciliation 延遲
- 錯誤率

### Health Checks

- **Liveness**: `/healthz` (port 8081)
- **Readiness**: `/readyz` (port 8081)

### 日誌

使用結構化日誌（zap）：

```go
log.Info("Creating Deployment", "name", deployment.Name)
log.Error(err, "Failed to reconcile", "viewer", viewer.Name)
```

## 擴展性設計

### 支援其他應用程式

PVCViewer 設計為可擴展的，可用於啟動其他類型的應用：

**範例：TensorBoard**

```yaml
apiVersion: kubeflow.org/v1alpha1
kind: PVCViewer
metadata:
  name: tensorboard-viewer
spec:
  pvc: training-logs
  podSpec:
    containers:
    - name: tensorboard
      image: tensorflow/tensorflow:latest
      command: ["tensorboard", "--logdir=/logs"]
      ports:
      - containerPort: 6006
      volumeMounts:
      - name: viewer-volume
        mountPath: /logs
  networking:
    targetPort: 6006
    basePrefix: "/tensorboard"
```

### 未來增強

1. **多 PVC 支援**：掛載多個 PVC
2. **訪問控制**：整合 Kubernetes RBAC
3. **資源配額**：CPU/Memory 限制
4. **自動清理**：基於時間的資源回收
5. **UI 整合**：提供 Web UI 管理介面

## 測試策略

### 單元測試

測試檔案位置：[`controllers/pvcviewer_controller_test.go`](controllers/pvcviewer_controller_test.go)

使用 Ginkgo/Gomega 框架進行測試。

### 整合測試

執行完整的協調循環測試：

```bash
make test
```

### 測試工具

測試輔助工具：[`controllers/test_utils.go`](controllers/test_utils.go)

## 參考資料

- [Kubernetes Operator Pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
- [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime)
- [Kubebuilder](https://book.kubebuilder.io/)
- [Istio VirtualService](https://istio.io/latest/docs/reference/config/networking/virtual-service/)
- [FileBrowser](https://filebrowser.org/)

## 版本歷史

- **v1alpha1** (2023) - 初始版本
  - 基本 PVCViewer 功能
  - RWO 調度支援
  - Webhook 整合
  - Istio VirtualService 支援

## 授權

Copyright 2023. Licensed under the Apache License, Version 2.0.

詳見 LICENSE 檔案。