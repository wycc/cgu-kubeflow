package http

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PVCViewerReconciler reconciles a PVCViewer object
type PVCViewerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

type RequestBody struct {
	Url     string `json:"url"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}
type Outgoing struct {
	Type    string `json:"type"`
	Format  string `json:"format"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CopyFileToPod streams a local file into a container in a pod via tar over exec.
func CopyFileToPod(config *rest.Config, clientset *kubernetes.Clientset,
	namespace, podName, containerName, localPath, targetDir string,
) error {
	// Pack local file into tar buffer
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)
	defer tw.Close()

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	fi, err := file.Stat()
	if err != nil {
		return err
	}

	hdr := &tar.Header{
		Name: filepath.Base(localPath),
		Mode: int64(fi.Mode().Perm()),
		Size: fi.Size(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, file); err != nil {
		return err
	}
	tw.Close()

	// Prepare exec request
	log.Printf("Copying to container: %s in pod %s/%s", containerName, namespace, podName)
	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   []string{"tar", "xmf", "-", "-C", targetDir},
			Stdin:     true, Stdout: true, Stderr: true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return err
	}

	// Stream the tar into the container
	return executor.Stream(remotecommand.StreamOptions{
		Stdin:  buf,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})
}

func (r *PVCViewerReconciler) getEditor(ctx context.Context, namespace string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "editor",
			Namespace: namespace,
		},
	}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment)
	return err
}

var fileHandler = func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return http.StatusMethodNotAllowed, nil
	}

	// 讀 body
	var body RequestBody
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Bad Request: "+err.Error(), http.StatusBadRequest)
		return http.StatusBadRequest, err
	}
	// 取得使用者 email（kubeflow 前端通常會在 request header 放 kubeflow-userid）
	emailAddress := r.Header.Get("kubeflow-userid")
	if emailAddress == "" {
		// 如果 header 裡沒有，提供一個預設值或回傳錯誤
		log.Printf("kubeflow-userid header not found")
		http.Error(w, "kubeflow-userid header is required", http.StatusBadRequest)
		return http.StatusBadRequest, nil
	}

	// Build Kubernetes client config (in-cluster)
	config, err := rest.InClusterConfig()
	if err != nil {
		http.Error(w, "Failed to create in-cluster k8s config: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		http.Error(w, "Failed to create k8s client: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// 使用 dynamic client 列出 Profiles CRD，找出 spec.owner.name == emailAddress 的 profile
	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		http.Error(w, "Failed to create dynamic client: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}
	gvr := schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "profiles"}
	list, err := dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{})
	if err != nil {
		http.Error(w, "Failed to list profiles: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}
	namespace := ""
	for _, item := range list.Items {
		ownerName, found, _ := unstructured.NestedString(item.Object, "spec", "owner", "name")
		if found && ownerName == emailAddress {
			namespace = item.GetName()
			break
		}
	}
	if namespace == "" {
		http.Error(w, "No profile or namespace found for user: "+emailAddress, http.StatusNotFound)
		return http.StatusNotFound, nil
	}
	log.Printf("Found namespace '%s' for user '%s'", namespace, emailAddress)

	// Write content to temporary local file
	if err := ioutil.WriteFile("/tmp/"+body.Name, []byte(body.Content), 0644); err != nil {
		http.Error(w, "Failed to write temp file: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// Copy file into target Pod
	// 透過檢查 deployment 的 replicas 來確認 notebook 是否正在運行
	deployment, err := clientset.AppsV1().Deployments(namespace).Get(r.Context(), "editor", metav1.GetOptions{})
	if err != nil {
		http.Error(w, "Failed to get editor deployment: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}

	// 如果 replicas 為 0，代表 notebook 已停止
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0 {
		log.Printf("Notebook deployment is scaled to 0 replicas. It's likely stopped.")
		// 這裡您可以選擇直接回傳錯誤，或嘗試去啟動它。
		// 若要啟動，仍需透過 patch Notebook CRD，無法單純修改 deployment。
		http.Error(w, "Notebook is not running. Please start it from the Kubeflow UI and retry.", http.StatusAccepted)
		return http.StatusAccepted, nil
	}

	// 檢查 Pod 狀態，確保有正在運行的 Pod
	pods, err := clientset.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: "app=editor", // 假設 deployment 使用此標籤
	})
	if err != nil {
		http.Error(w, "Failed to list editor pods: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}

	podName := ""
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podName = pod.Name
			break
		}
	}

	if podName == "" {
		http.Error(w, "No running editor pod found. Please wait for the pod to start and retry.", http.StatusAccepted)
		return http.StatusAccepted, nil
	}

	err = CopyFileToPod(config, clientset,
		namespace, // namespace
		podName,   // pod name
		"editor",  // container name
		"/tmp/"+body.Name,
		filepath.Dir(body.Path),
	)
	if err != nil {
		http.Error(w, "Failed to copy to pod: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}
	// Respond success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("File copied into pod"))
	return http.StatusOK, nil
}
