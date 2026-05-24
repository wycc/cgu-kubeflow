package controllers

// SSHServiceLabel 是 Notebook 上用來啟用 SSH Service 的 Label Key。
// 設定值為 "true" 時，Controller 會自動為此 Notebook 建立 SSH NodePort Service。
// 範例：cgu.kubeflow.org/sshservice: "true"
const SSHServiceLabel = "cgu.kubeflow.org/sshservice"

// SSHNodePortAnnotation 是 Controller 回寫給 Notebook 的 Annotation Key，
// 記錄系統指派的 NodePort，供 Kubeflow UI 讀取並顯示連線資訊給使用者。
// 範例：cgu.kubeflow.org/ssh-nodeport: "31234"
const SSHNodePortAnnotation = "cgu.kubeflow.org/ssh-nodeport"

// IstioExcludePortAnnotation 是加在 StatefulSet Pod Template 上的 Istio Annotation，
// 讓 Istio Sidecar（Envoy）略過對 SSH 入站流量的攔截。
const IstioExcludePortAnnotation = "traffic.sidecar.istio.io/excludeInboundPorts"

// SSHContainerPort 是 Notebook 容器內的 SSH 監聽埠。
const SSHContainerPort = int32(22)

// SSHServicePort 是 SSH Service 對外（ClusterIP）使用的埠號。
const SSHServicePort = int32(2222)

// IstioExcludePortValue 是要排除的 Istio 攔截埠號字串，對應 SSHContainerPort。
const IstioExcludePortValue = "22"
