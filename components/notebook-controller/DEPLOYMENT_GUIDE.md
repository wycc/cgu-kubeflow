# Notebook Controller TCP Port 8000 部署指南

## 🚀 Docker Image 資訊

**Image 名稱:** `notebook-controller:tcp8000-v1.5.0-rc.0-465-geccac2d9-dirty`  
**Image ID:** `f8a8e5d4472a`  
**大小:** 74.4MB  
**構建時間:** 2025-08-01

## 📋 功能摘要

此版本的 Notebook Controller 包含以下新功能：

- ✅ **端口 8000 TCP 支持**: 每個 notebook 自動暴露端口 8000
- ✅ **集群範圍訪問**: 允許集群內任何 Pod 訪問端口 8000
- ✅ **Istio AuthorizationPolicy**: 自動創建授權策略
- ✅ **向後兼容**: 不影響現有功能

## 🔧 部署步驟

### 1. 推送 Docker Image（如需要）

```bash
# 標記 image 為你的 registry
sudo docker tag notebook-controller:tcp8000-v1.5.0-rc.0-465-geccac2d9-dirty your-registry/notebook-controller:tcp8000-latest

# 推送到 registry
sudo docker push your-registry/notebook-controller:tcp8000-latest
```

### 2. 更新部署配置

修改 `config/base/kustomization.yaml` 中的 image 設定：

```yaml
images:
- name: notebook-controller
  newName: your-registry/notebook-controller
  newTag: tcp8000-latest
```

### 3. 部署到 Kubernetes

```bash
# 確保啟用 Istio
export USE_ISTIO=true

# 部署控制器
kubectl apply -k config/base/
```

### 4. 驗證部署

```bash
# 檢查控制器 Pod
kubectl get pods -n kubeflow

# 檢查控制器日誌
kubectl logs -n kubeflow deployment/notebook-controller-deployment

# 創建測試 notebook
kubectl apply -f config/samples/_v1beta1_notebook_with_tcp8000.yaml
```

## 🧪 測試新功能

### 1. 創建測試 Notebook

```bash
kubectl apply -f config/samples/_v1beta1_notebook_with_tcp8000.yaml
```

### 2. 檢查生成的資源

```bash
# 檢查 Service 端口
kubectl get svc notebook-tcp8000-sample -o yaml

# 檢查 AuthorizationPolicy
kubectl get authorizationpolicy -n kubeflow-user-example-com

# 檢查 Pod 端口
kubectl get pod notebook-tcp8000-sample-0 -o jsonpath='{.spec.containers[0].ports}'
```

### 3. 測試連接

```bash
# 從其他 Pod 測試連接
kubectl run test-pod --image=curlimages/curl --rm -it -- sh

# 在 test-pod 內執行
curl http://notebook-tcp8000-sample.kubeflow-user-example-com.svc.cluster.local:8000
```

## 📊 預期結果

### Service 配置
```yaml
spec:
  ports:
  - name: http-notebook-tcp8000-sample
    port: 80
    targetPort: 8888
    protocol: TCP
  - name: tcp-8000-notebook-tcp8000-sample
    port: 8000
    targetPort: 8000
    protocol: TCP
```

### AuthorizationPolicy 配置
```yaml
spec:
  action: ALLOW
  selector:
    matchLabels:
      statefulset: notebook-tcp8000-sample
  rules:
  - to:
    - operation:
        ports: ["8000"]
```

## 🔍 故障排除

### 常見問題

1. **AuthorizationPolicy 未創建**
   - 確認 `USE_ISTIO=true` 環境變數已設定
   - 檢查控制器日誌是否有錯誤

2. **端口 8000 無法訪問**
   - 檢查 Service 配置是否包含端口 8000
   - 確認 Pod 已正確啟動並暴露端口 8000

3. **權限錯誤**
   - 確認 RBAC 配置已更新
   - 檢查控制器是否有 AuthorizationPolicy 管理權限

### 檢查命令

```bash
# 檢查控制器權限
kubectl auth can-i create authorizationpolicies --as=system:serviceaccount:kubeflow:notebook-controller-service-account

# 檢查 Istio 狀態
kubectl get pods -n istio-system

# 檢查網路連接
kubectl exec -it test-pod -- nslookup notebook-tcp8000-sample.kubeflow-user-example-com.svc.cluster.local
```

## 📚 相關文件

- [功能文檔](docs/TCP_PORT_8000_FEATURE.md)
- [示例配置](config/samples/_v1beta1_notebook_with_tcp8000.yaml)
- [AuthorizationPolicy 示例](hack/example_tcp8000_authorization_policy.yaml)

## 🔄 回滾步驟

如需回滾到原版本：

```bash
# 使用原始 image
kubectl set image deployment/notebook-controller-deployment manager=original-image:tag -n kubeflow

# 或重新部署原始配置
kubectl apply -k config/base/
```

## 📞 支援

如遇到問題，請檢查：
1. 控制器日誌
2. Istio 配置
3. 網路策略設定
4. RBAC 權限

---
**注意**: 此版本需要 Istio 支持才能獲得完整的安全功能。