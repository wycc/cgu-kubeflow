# Kubeflow Notebook SSH Controller

## 概觀

`notebook-ssh-controller` 是一個 Kubernetes 控制器，專為 Kubeflow 設計。它的主要功能是監控在 `kubeflow` 命名空間中建立的 `Notebook` 自訂資源。當一個新的 `Notebook` 被建立或更新時，這個控制器會自動為其建立一個 `NodePort` 類型的 `Service`，以便將 `Notebook` 容器內的 `sshd` 服務暴露到 Kubernetes 叢集外部。

這個控制器解決了在 Kubeflow Notebook 中運行 `sshd` 服務時，需要手動設定網路連線的問題，讓使用者可以更方便地透過 SSH 連線到他們的 Notebook 環境。

## 功能

*   **自動建立 `NodePort` Service**：監控 `Notebook` 資源，並為其第二個容器埠口（預設為 `sshd`）建立一個 `NodePort` Service。
*   **服務生命週期管理**：當 `Notebook` 被刪除時，自動清理對應的 `Service`。
*   **狀態同步**：確保 `Service` 的設定與 `Notebook` 的狀態保持一致。

## 開始使用

### 先決條件

*   一個正在運行的 Kubernetes 叢集。
*   已安裝 Kubeflow，特別是 `notebook-controller`。
*   `kubectl` 已設定並連線到您的叢集。
*   已安裝 Docker。
*   已安裝 Go (版本 1.23 或更高)。
*   已安裝 `kustomize`。

### 建置

1.  **Clone 專案**

    ```bash
    git clone https://github.com/kubeflow/notebook-ssh-controller.git
    cd notebook-ssh-controller
    ```

2.  **整理依賴**

    ```bash
    go mod tidy
    ```

3.  **建置 Docker 映像**

    使用 `Makefile` 中的指令來建置控制器映像。您可以自訂映像名稱和標籤。

    ```bash
    make docker-build IMG=<your-registry>/notebook-ssh-controller:latest
    ```

4.  **推送 Docker 映像**

    將建置好的映像推送到您的容器映像庫。

    ```bash
    docker push <your-registry>/notebook-ssh-controller:latest
    ```

### 部署

1.  **更新部署配置**

    在部署之前，您需要更新位於 `config/manager/manager.yaml` 的部署配置，將映像名稱改為您剛剛推送的映像。

2.  **部署控制器**

    使用 `make deploy` 指令將控制器部署到您的 Kubernetes 叢集。這會建立所有必要的資源，包括 `Deployment`、`ServiceAccount`、`ClusterRole` 和 `ClusterRoleBinding`。

    ```bash
    make deploy IMG=<your-registry>/notebook-ssh-controller:latest
    ```

### 驗證

1.  **檢查 Controller Pod**

    確認 `notebook-ssh-controller` 的 pod 是否正在 `notebook-ssh-controller-system` 命名空間中運行。

    ```bash
    kubectl get pods -n notebook-ssh-controller-system
    ```

2.  **建立一個新的 Notebook**

    在 Kubeflow UI 中建立一個新的 Notebook，並確保其容器規格中有兩個埠口，其中第二個是 SSH 埠。

3.  **檢查 Service**

    在 Notebook 建立後，檢查是否有名為 `<notebook-name>-ssh` 的 `Service` 被自動建立在與 Notebook 相同的命名空間中。

    ```bash
    kubectl get service -n <notebook-namespace>
    ```

    您應該會看到一個 `NodePort` 類型的 Service，並可以從輸出中取得節點埠。

## 開發

如果您想為這個控制器貢獻，請參考以下指令：

*   `make run`：在本地機器上以開發模式運行控制器。
*   `make test`：運行單元測試。

## 未來功能

### 在 Kubeflow UI 中整合 SSH 開關

一個理想的增強功能是在 Kubeflow 的 Notebook 建立頁面中，直接提供一個「啟用 SSH」的選項。這將允許使用者為每個 Notebook 獨立地決定是否要開啟 SSH 連線。

要實現這個功能，需要修改 Kubeflow 的前端，讓「啟用 SSH」的選項能夠動態地在 Notebook 的容器規格中，加入或移除 `sshd` 的埠口設定。

*   **修改 Kubeflow 前端**：
    *   修改 Kubeflow 的 Notebook 管理介面，在建立和編輯 Notebook 的表單中，加入一個「啟用 SSH」的核取方塊。
    *   當使用者勾選該選項時，前端程式碼會在送出給後端的 `Notebook` 物件中，自動加入一個代表 `sshd` 服務的 `containerPort`（例如，埠口 `22`）。
    *   當使用者取消勾選時，則移除該埠口設定。

這個方法的優點是不需要修改 Kubeflow 的 Notebook CRD，只需要修改前端介面即可。`notebook-ssh-controller` 不需要做任何變更，它會自動根據是否存在第二個埠口，來建立或刪除對應的 SSH `Service`。
