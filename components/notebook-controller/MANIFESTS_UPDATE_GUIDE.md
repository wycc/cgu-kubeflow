# Manifests 更新指南

## 📋 需要修正的 Manifests 文件

基於我們添加的 TCP 端口 8000 功能和 Istio AuthorizationPolicy 支持，以下是需要檢查和可能需要更新的 manifests 文件：

## ✅ 已經正確更新的文件

### 1. **RBAC 權限** - `config/rbac/role.yaml`
```yaml
# 已添加 AuthorizationPolicy 權限
- apiGroups:
  - security.istio.io
  resources:
  - authorizationpolicies
  verbs:
  - '*'
```
**狀態**: ✅ 已更新

## 🔍 需要檢查的文件

### 2. **控制器部署** - `config/manager/manager.yaml`
**當前狀態**: 使用現有配置
**建議**: 如果需要，可以更新 image 標籤指向新構建的版本

**可選更新**:
```yaml
# 在 config/base/kustomization.yaml 中更新 image
images:
- name: docker.io/kubeflownotebookswg/notebook-controller
  newName: notebook-controller  # 或您的 registry
  newTag: tcp8000-v1.5.0-rc.0-465-geccac2d9-dirty
```

### 3. **環境變數配置**
**當前狀態**: 已包含 `USE_ISTIO` 環境變數
**狀態**: ✅ 無需修改

## 📝 CRD 文件狀態

### 4. **CustomResourceDefinition** - `config/crd/bases/kubeflow.org_notebooks.yaml`
**狀態**: ✅ 無需修改
**原因**: 我們的修改是在控制器邏輯層面，不涉及 CRD schema 變更

## 🚀 部署建議

### 方法 1: 使用現有 manifests（推薦）
```bash
# 直接使用現有配置部署
kubectl apply -k config/base/
```

### 方法 2: 使用新構建的 image
```bash
# 1. 更新 image 標籤
sed -i 's|newTag: v1.8.0|newTag: tcp8000-v1.5.0-rc.0-465-geccac2d9-dirty|' config/base/kustomization.yaml
sed -i 's|newName: docker.io/kubeflownotebookswg/notebook-controller|newName: notebook-controller|' config/base/kustomization.yaml

# 2. 部署
kubectl apply -k config/base/
```

### 方法 3: 直接使用 make 部署
```bash
# 設定環境變數
export IMG=notebook-controller
export TAG=tcp8000-v1.5.0-rc.0-465-geccac2d9-dirty

# 部署（會自動更新 image 配置）
make deploy
```

## 🔧 驗證部署

### 1. 檢查控制器 Pod
```bash
kubectl get pods -n kubeflow -l app=notebook-controller
```

### 2. 檢查 RBAC 權限
```bash
# 檢查 AuthorizationPolicy 權限
kubectl auth can-i create authorizationpolicies --as=system:serviceaccount:kubeflow:notebook-controller-service-account
```

### 3. 測試功能
```bash
# 創建測試 notebook
kubectl apply -f config/samples/_v1beta1_notebook_with_tcp8000.yaml

# 檢查生成的資源
kubectl get svc,authorizationpolicy -n kubeflow-user-example-com
```

## 📊 修改摘要

| 文件 | 狀態 | 說明 |
|------|------|------|
| `config/rbac/role.yaml` | ✅ 已更新 | 添加 AuthorizationPolicy 權限 |
| `config/manager/manager.yaml` | ✅ 無需修改 | 環境變數配置已足夠 |
| `config/base/kustomization.yaml` | 🔄 可選更新 | 可更新 image 標籤 |
| `config/crd/bases/kubeflow.org_notebooks.yaml` | ✅ 無需修改 | CRD schema 無變更 |

## ⚠️ 重要注意事項

1. **RBAC 權限**: 已正確添加 `security.istio.io/authorizationpolicies` 權限
2. **向後兼容**: 所有修改都保持向後兼容性
3. **Istio 依賴**: 功能需要 `USE_ISTIO=true` 環境變數
4. **自動功能**: 端口 8000 會自動添加到所有新創建的 notebook

## 🔄 如果 controller-gen 工具修復

如果 `make manifests` 工具修復，可以重新生成：
```bash
# 重新生成 manifests（當工具修復後）
make manifests

# 檢查生成的 RBAC 配置
git diff config/rbac/role.yaml
```

## 📞 故障排除

如果遇到權限問題：
1. 確認 RBAC 配置已正確應用
2. 檢查控制器日誌中的權限錯誤
3. 驗證 Istio 是否正確安裝和配置

---
**結論**: 主要的 manifests 文件已經正確配置，可以直接部署使用。