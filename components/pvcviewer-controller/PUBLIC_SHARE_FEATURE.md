# PVCViewer 公開分享功能

## 概述

PVCViewer 現在支援公開分享功能，允許 `/share/` 路徑下的內容被公開存取，無需 Kubeflow 身份驗證。

## 特性

- **自動啟用**：當 `networking` 配置存在時，公開分享功能會自動啟用
- **無需修改 CRD**：使用現有的配置欄位，不需要額外的設定
- **安全設計**：
  - 只允許 `/share/` 路徑公開存取
  - 限制 HTTP 方法為 GET、HEAD、OPTIONS（只讀）
  - 雙層授權檢查（Gateway + Backend）
- **自動清理**：使用 Finalizer 確保所有資源在刪除時被正確清理

## 使用方法

### 1. 創建 PVCViewer

```yaml
apiVersion: kubeflow.org/v1alpha1
kind: PVCViewer
metadata:
  name: my-pvcviewer
  namespace: kubeflow-user-example-com
spec:
  pvc: my-workspace-pvc
  networking:
    targetPort: 8080
    basePrefix: /pvcviewers
```

### 2. 存取路徑

創建後，將自動生成以下存取路徑：

- **私有路徑**（需要認證）：
  ```
  https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/
  ```

- **公開路徑**（無需認證）：
  ```
  https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/
  ```

## 自動創建的資源

當 PVCViewer 配置了 `networking` 時，Controller 會自動創建以下額外資源：

### 1. VirtualService (Share)
- **名稱**：`pvcviewer-share-{name}`
- **Namespace**：PVCViewer namespace
- **用途**：為 `/share/` 路徑創建路由規則

### 2. EnvoyFilter
- **名稱**：`bypass-auth-pvcviewer-share-{namespace}-{name}`
- **Namespace**：`istio-system`
- **用途**：繞過 Kubeflow 的身份驗證

### 3. AuthorizationPolicy (Gateway)
- **名稱**：`allow-pvcviewer-share-{namespace}-{name}-gw`
- **Namespace**：`istio-system`
- **用途**：在 Gateway 層允許公開存取

### 4. AuthorizationPolicy (Backend)
- **名稱**：`allow-pvcviewer-share-{name}-inbound`
- **Namespace**：PVCViewer namespace
- **用途**：在後端 Pod 層限制 HTTP 方法和路徑

## 安全考量

### 允許的 HTTP 方法

公開分享**只允許**以下 HTTP 方法：
- `GET` - 讀取檔案
- `HEAD` - 獲取檔案元數據
- `OPTIONS` - CORS 預檢請求

**不允許**：`POST`、`PUT`、`DELETE`、`PATCH` 等修改操作

### 路徑隔離

- 只有 `/share/` 路徑下的內容可公開存取
- 其他路徑仍需要 Kubeflow 身份驗證
- 使用精確的路徑前綴匹配，避免路徑遍歷攻擊

### 雙層授權

1. **Gateway 層**：Istio Gateway 的 AuthorizationPolicy 進行初步過濾
2. **Backend 層**：Pod 的 AuthorizationPolicy 進行二次檢查

## 驗證功能

### 1. 檢查資源創建

```bash
# 檢查 PVCViewer
kubectl get pvcviewer -n kubeflow-user-example-com

# 檢查 VirtualService (share)
kubectl get virtualservice -n kubeflow-user-example-com pvcviewer-share-my-pvcviewer

# 檢查 EnvoyFilter
kubectl get envoyfilter -n istio-system bypass-auth-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer

# 檢查 AuthorizationPolicy (Gateway)
kubectl get authorizationpolicy -n istio-system allow-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer-gw

# 檢查 AuthorizationPolicy (Backend)
kubectl get authorizationpolicy -n kubeflow-user-example-com allow-pvcviewer-share-my-pvcviewer-inbound

# 檢查 Finalizer
kubectl get pvcviewer my-pvcviewer -n kubeflow-user-example-com -o jsonpath='{.metadata.finalizers}'
```

### 2. 測試公開存取

```bash
# 測試公開路徑（應該成功，無需認證）
curl -I https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/test.txt

# 測試私有路徑（應該重定向到登入頁或返回 401）
curl -I https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/test.txt
```

### 3. 測試 HTTP 方法限制

```bash
# GET 應該成功
curl -X GET https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/file.txt

# POST 應該被拒絕 (403 Forbidden)
curl -X POST https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/file.txt

# DELETE 應該被拒絕 (403 Forbidden)
curl -X DELETE https://your-domain/pvcviewers/kubeflow-user-example-com/my-pvcviewer/share/file.txt
```

## 資源清理

當刪除 PVCViewer 時：

1. Controller 檢測到刪除請求
2. Finalizer 觸發清理邏輯
3. 清理 `istio-system` 中的資源：
   - EnvoyFilter
   - AuthorizationPolicy (Gateway)
4. 移除 Finalizer
5. Kubernetes 自動清理其他資源（有 OwnerReference 的資源）

驗證清理：
```bash
# 刪除 PVCViewer
kubectl delete pvcviewer my-pvcviewer -n kubeflow-user-example-com

# 驗證 istio-system 資源已清理
kubectl get envoyfilter -n istio-system | grep my-pvcviewer
kubectl get authorizationpolicy -n istio-system | grep my-pvcviewer
```

## 故障排除

### 問題 1：公開路徑無法存取

**可能原因**：
- Networking 配置未設定
- Istio Gateway 配置錯誤
- 資源創建失敗

**檢查步驟**：
```bash
# 1. 檢查 PVCViewer 狀態
kubectl describe pvcviewer my-pvcviewer -n kubeflow-user-example-com

# 2. 檢查 Controller 日誌
kubectl logs -n kubeflow deployment/pvcviewer-controller-manager -c manager

# 3. 檢查所有相關資源是否存在
kubectl get virtualservice,envoyfilter,authorizationpolicy --all-namespaces | grep my-pvcviewer
```

### 問題 2：資源未被清理

**可能原因**：
- Finalizer 處理失敗
- Controller 權限不足

**解決方法**：
```bash
# 1. 檢查 Finalizer
kubectl get pvcviewer my-pvcviewer -n kubeflow-user-example-com -o yaml | grep finalizers -A 5

# 2. 手動清理（如果需要）
kubectl delete envoyfilter bypass-auth-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer -n istio-system
kubectl delete authorizationpolicy allow-pvcviewer-share-kubeflow-user-example-com-my-pvcviewer-gw -n istio-system

# 3. 移除 Finalizer（謹慎使用）
kubectl patch pvcviewer my-pvcviewer -n kubeflow-user-example-com -p '{"metadata":{"finalizers":[]}}' --type=merge
```

### 問題 3：權限錯誤

**症狀**：Controller 日誌顯示權限不足錯誤

**解決方法**：
```bash
# 1. 檢查 ClusterRole
kubectl get clusterrole pvcviewer-controller-role -o yaml

# 2. 確認包含以下權限
# - networking.istio.io/envoyfilters
# - security.istio.io/authorizationpolicies

# 3. 如果缺少權限，重新應用 RBAC
kubectl apply -f config/rbac/role.yaml
```

## 開發和測試

### 本地測試

```bash
# 1. 安裝 CRD
make install

# 2. 運行 Controller
make run

# 3. 在另一個終端創建測試資源
kubectl apply -f config/samples/pvcviewer_with_public_share.yaml

# 4. 觀察日誌
# Controller 日誌會顯示：
# - "Adding public share finalizer"
# - "Creating Share VirtualService"
# - "Creating Share EnvoyFilter"
# - "Creating Share AuthorizationPolicy (Gateway)"
# - "Creating Share AuthorizationPolicy (Backend)"
```

### 單元測試

```bash
# 運行測試
make test

# 運行特定測試
go test ./controllers -v -run TestPublicShare
```

## 參考文檔

- [設計文檔](PUBLIC_SHARE_DESIGN.md) - 完整的架構設計和實作細節
- [主要設計文檔](DESIGN.md) - PVCViewer 整體設計
- [範例配置](config/samples/pvcviewer_with_public_share.yaml) - 使用範例