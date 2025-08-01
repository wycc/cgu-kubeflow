# TCP Port 8000 Feature for Notebook Controller

## 概述

此功能為 Kubeflow Notebook Controller 添加了對端口 8000 的 TCP 訪問支持，允許集群內的任何 Pod 通過端口 8000 與 notebook 進行通信。同時，通過 Istio AuthorizationPolicy 提供安全的訪問控制。

## 功能特性

- **自動端口暴露**: 每個 notebook 自動暴露端口 8000 用於 TCP 通信
- **服務配置**: Service 自動包含端口 8000 的映射
- **Istio 集成**: 當啟用 Istio 時，自動創建 AuthorizationPolicy
- **集群範圍訪問**: 允許集群內任何 Pod 訪問端口 8000
- **向後兼容**: 不影響現有的 HTTP 流量和功能

## 架構變更

### 1. StatefulSet 修改
- 容器端口配置中添加端口 8000
- 端口名稱: `tcp-8000`
- 協議: TCP

### 2. Service 修改
- 添加端口 8000 的服務端口
- 端口名稱: `tcp-8000-{notebook-name}`
- 目標端口: 8000

### 3. AuthorizationPolicy (Istio)
- 自動為每個 notebook 創建授權策略
- 策略名稱: `notebook-{namespace}-{name}-tcp-8000`
- 允許集群內任何 Pod 訪問端口 8000

## 使用方法

### 環境變數配置

確保啟用 Istio 功能：
```bash
export USE_ISTIO=true
```

### 創建 Notebook

使用標準的 Notebook CR，控制器會自動添加端口 8000 支持：

```yaml
apiVersion: kubeflow.org/v1beta1
kind: Notebook
metadata:
  name: my-notebook
  namespace: kubeflow-user-example-com
spec:
  template:
    spec:
      containers:
        - name: my-notebook
          image: kubeflownotebookswg/jupyter:latest
          # 端口 8000 會自動添加
```

### 從其他 Pod 訪問

集群內任何命名空間的 Pod 都可以通過以下方式訪問：

```bash
# 通過完整服務名稱訪問（推薦，跨命名空間訪問）
curl http://my-notebook.kubeflow-user-example-com.svc.cluster.local:8000

# 或者使用簡短名稱（僅限同一命名空間內）
curl http://my-notebook:8000
```

## 生成的資源

### Service 示例
```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-notebook
  namespace: kubeflow-user-example-com
spec:
  ports:
  - name: http-my-notebook
    port: 80
    targetPort: 8888
    protocol: TCP
  - name: tcp-8000-my-notebook
    port: 8000
    targetPort: 8000
    protocol: TCP
  selector:
    statefulset: my-notebook
```

### AuthorizationPolicy 示例
```yaml
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: notebook-kubeflow-user-example-com-my-notebook-tcp-8000
  namespace: kubeflow-user-example-com
spec:
  action: ALLOW
  selector:
    matchLabels:
      statefulset: my-notebook
  rules:
  - to:
    - operation:
        ports: ["8000"]
```

## 安全考量

1. **集群範圍訪問**: AuthorizationPolicy 允許集群內任何 Pod 訪問端口 8000
2. **端口特定**: 授權策略只針對端口 8000，不影響其他端口
3. **Istio 依賴**: 安全功能依賴於 Istio 的啟用
4. **網路策略**: 如需更嚴格的訪問控制，建議配合 Kubernetes NetworkPolicy

## 故障排除

### 檢查服務端口
```bash
kubectl get svc my-notebook -o yaml
```

### 檢查 AuthorizationPolicy
```bash
kubectl get authorizationpolicy -n kubeflow-user-example-com
```

### 檢查 Pod 端口
```bash
kubectl get pod my-notebook-0 -o jsonpath='{.spec.containers[0].ports}'
```

### 測試連接
```bash
# 從任何命名空間的 Pod 內測試
kubectl exec -it test-pod -n any-namespace -- curl http://my-notebook.kubeflow-user-example-com.svc.cluster.local:8000

# 從同命名空間的 Pod 內測試
kubectl exec -it test-pod -n kubeflow-user-example-com -- curl http://my-notebook:8000
```

## 限制

1. 只支援 TCP 協議
2. 端口 8000 是固定的，不可配置
3. 需要啟用 Istio 才能獲得完整的安全功能
4. 允許集群內任何 Pod 訪問（如需限制請使用額外的 NetworkPolicy）

## 相關文件

- 示例配置: `config/samples/_v1beta1_notebook_with_tcp8000.yaml`
- AuthorizationPolicy 示例: `hack/example_tcp8000_authorization_policy.yaml`
- 控制器代碼: `controllers/notebook_controller.go`