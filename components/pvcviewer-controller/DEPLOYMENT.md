# PVCViewer Controller 部署說明

## Docker Image 資訊

**Image**: `cguaicadmin/pvcviewer-controller:v1.5.0-rc.0-428-g1f67cf82-dirty`  
**Digest**: `sha256:5938a606dec8fabeb81d28af77d302e11c1729845fe7f97ac6b9a3316cee2539`  
**Size**: 3023 bytes (manifest)

## 包含的新功能

此版本包含**公開分享功能**，允許 `/share/` 路徑下的內容無需認證即可存取。

### 主要變更

1. ✅ 自動創建公開分享資源（VirtualService、EnvoyFilter、AuthorizationPolicy）
2. ✅ Finalizer 機制確保資源清理
3. ✅ 雙層安全授權（Gateway + Backend）
4. ✅ HTTP 方法限制（只允許 GET、HEAD、OPTIONS）
5. ✅ 更新的 RBAC 權限

## 部署步驟

### 1. 更新 CRD

```bash
kubectl apply -f config/crd/bases/kubeflow.org_pvcviewers.yaml
```

### 2. 更新 RBAC

```bash
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml
```

### 3. 更新 Controller Deployment

編輯 Controller deployment 使用新的 image：

```bash
kubectl set image deployment/pvcviewer-controller-manager \
  manager=cguaicadmin/pvcviewer-controller:v1.5.0-rc.0-428-g1f67cf82-dirty \
  -n kubeflow
```

或者使用 kustomize：

```bash
cd config/base
kustomize edit set image pvcviewer-controller=cguaicadmin/pvcviewer-controller:v1.5.0-rc.0-428-g1f67cf82-dirty
kubectl apply -k config/base
```

### 4. 驗證部署

```bash
# 檢查 Controller 狀態
kubectl get deployment -n kubeflow pvcviewer-controller-manager
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager --tail=100

# 檢查 RBAC
kubectl get clusterrole pvcviewer-controller-role -o yaml | grep -A 5 "envoyfilters\|authorizationpolicies"
```

## 測試公開分享功能

### 1. 創建測試 PVCViewer

```bash
kubectl apply -f config/samples/pvcviewer_with_public_share.yaml
```

### 2. 檢查創建的資源

```bash
# 檢查 PVCViewer
kubectl get pvcviewer -n kubeflow-user-example-com

# 檢查 Finalizer
kubectl get pvcviewer pvcviewer-sample-with-share -n kubeflow-user-example-com \
  -o jsonpath='{.metadata.finalizers}'

# 檢查公開分享資源
kubectl get virtualservice -n kubeflow-user-example-com pvcviewer-share-pvcviewer-sample-with-share
kubectl get envoyfilter -n istio-system | grep pvcviewer-share
kubectl get authorizationpolicy -n istio-system | grep pvcviewer-share
kubectl get authorizationpolicy -n kubeflow-user-example-com | grep pvcviewer-share
```

### 3. 測試公開存取

```bash
# 測試公開路徑（應該成功，無需認證）
curl -I https://your-domain/pvcviewers/kubeflow-user-example-com/pvcviewer-sample-with-share/share/

# 測試私有路徑（應該需要認證）
curl -I https://your-domain/pvcviewers/kubeflow-user-example-com/pvcviewer-sample-with-share/

# 測試 HTTP 方法限制
# GET 應該成功
curl -X GET https://your-domain/pvcviewers/kubeflow-user-example-com/pvcviewer-sample-with-share/share/file.txt

# POST 應該被拒絕（403 Forbidden）
curl -X POST https://your-domain/pvcviewers/kubeflow-user-example-com/pvcviewer-sample-with-share/share/file.txt
```

## 回滾步驟

如果需要回滾到之前的版本：

```bash
# 回滾 Deployment
kubectl rollout undo deployment/pvcviewer-controller-manager -n kubeflow

# 或指定特定版本
kubectl rollout undo deployment/pvcviewer-controller-manager -n kubeflow --to-revision=<revision>

# 檢查回滾狀態
kubectl rollout status deployment/pvcviewer-controller-manager -n kubeflow
```

## 故障排除

### Controller 無法啟動

檢查日誌：
```bash
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager --tail=100
```

常見問題：
- RBAC 權限不足：確認 ClusterRole 包含 EnvoyFilter 和 AuthorizationPolicy 權限
- Image pull 錯誤：檢查 image 名稱和 registry 存取權限

### 公開分享資源未創建

檢查：
```bash
# 確認 Networking 配置存在
kubectl get pvcviewer <name> -n <namespace> -o jsonpath='{.spec.networking}'

# 檢查 Controller 日誌
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager | grep "public share"
```

### 資源清理失敗

如果 PVCViewer 無法刪除（卡在 Terminating）：

```bash
# 檢查 Finalizer
kubectl get pvcviewer <name> -n <namespace> -o jsonpath='{.metadata.finalizers}'

# 檢查 Controller 日誌
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager | grep "cleanup"

# 如果必要，手動清理 istio-system 資源
kubectl delete envoyfilter bypass-auth-pvcviewer-share-<namespace>-<name> -n istio-system
kubectl delete authorizationpolicy allow-pvcviewer-share-<namespace>-<name>-gw -n istio-system

# 移除 Finalizer（最後手段）
kubectl patch pvcviewer <name> -n <namespace> \
  -p '{"metadata":{"finalizers":[]}}' --type=merge
```

## 監控

### 重要指標

監控以下內容確保功能正常：

1. **Controller 健康狀態**
   ```bash
   kubectl get pods -n kubeflow -l control-plane=controller-manager
   ```

2. **資源創建成功率**
   ```bash
   # 檢查所有 PVCViewer 的狀態
   kubectl get pvcviewer --all-namespaces -o wide
   ```

3. **公開分享資源數量**
   ```bash
   # EnvoyFilters
   kubectl get envoyfilter -n istio-system | grep bypass-auth-pvcviewer-share | wc -l
   
   # AuthorizationPolicies
   kubectl get authorizationpolicy -n istio-system | grep allow-pvcviewer-share | wc -l
   ```

### 日誌關鍵字

監控這些日誌訊息：
- "Adding public share finalizer"
- "Creating Share VirtualService"
- "Creating Share EnvoyFilter"
- "Creating Share AuthorizationPolicy"
- "Cleaning up istio-system resources"
- "Failed to" (錯誤訊息)

## 相關文檔

- [公開分享功能說明](PUBLIC_SHARE_FEATURE.md)
- [設計文檔](PUBLIC_SHARE_DESIGN.md)
- [主要設計文檔](DESIGN.md)
- [使用範例](config/samples/pvcviewer_with_public_share.yaml)

## 支援

如有問題，請：
1. 檢查 Controller 日誌
2. 查看相關資源的事件：`kubectl describe pvcviewer <name> -n <namespace>`
3. 參考故障排除章節
4. 聯繫開發團隊