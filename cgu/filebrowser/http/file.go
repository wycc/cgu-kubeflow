package http

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

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
	var namespace = strings.Split(body.Url, "/")[4]

	// Build Kubernetes client config (in-cluster or kubeconfig)
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = clientcmd.BuildConfigFromFlags("", "/path/to/kubeconfig")
		if err != nil {
			http.Error(w, "Failed to load kubeconfig: "+err.Error(), http.StatusInternalServerError)
			return http.StatusInternalServerError, err
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		http.Error(w, "Failed to create k8s client: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// Write content to temporary local file
	if err := ioutil.WriteFile("/tmp/"+body.Name, []byte(body.Content), 0644); err != nil {
		http.Error(w, "Failed to write temp file: "+err.Error(), http.StatusInternalServerError)
		return http.StatusInternalServerError, err
	}

	// Copy file into target Pod
	// 嘗試 patch Kubeflow Notebook CRD 來啟動 notebook
	// 取得 Notebook CRD 物件
	notebookRes := clientset.RESTClient().
		Get().
		AbsPath("/apis/kubeflow.org/v1/namespaces/" + namespace + "/notebooks/editor")
	notebookRaw, err := notebookRes.Do(r.Context()).Raw()
	if err != nil {
		http.Error(w, "Failed to get notebook CRD: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}
	var notebookObj map[string]interface{}
	if err := json.Unmarshal(notebookRaw, &notebookObj); err != nil {
		http.Error(w, "Failed to parse notebook CRD: "+err.Error(), http.StatusBadGateway)
		return http.StatusBadGateway, err
	}
	annotations := notebookObj["metadata"].(map[string]interface{})["annotations"].(map[string]interface{})
	if val, ok := annotations["kubeflow-resource-stopped"]; ok && val != nil {
		// Notebook is stopped, patch to remove annotation to start it
		log.Printf("Notebook is stopped, patching to start it: %s", body.Name)
		nbRest := clientset.RESTClient()
		patchMap := map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					"kubeflow-resource-stopped": nil,
				},
			},
		}
		patchBody, _ := json.Marshal(patchMap)
		res := nbRest.Patch("merge").
			AbsPath("/apis/kubeflow.org/v1/namespaces/"+namespace+"/notebooks/editor").
			SetHeader("Content-Type", "application/merge-patch+json").
			Body(patchBody).
			Do(r.Context())
		if res.Error() != nil {
			http.Error(w, "Failed to patch notebook: "+res.Error().Error(), http.StatusBadGateway)
			return http.StatusBadGateway, res.Error()
		}
		http.Error(w, "Notebook is not running. Starting it, please retry later.", http.StatusAccepted)
		return http.StatusAccepted, nil
	}

	err = CopyFileToPod(config, clientset,
		namespace,  // namespace
		"editor-0", // pod name
		"editor",   // container name
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
