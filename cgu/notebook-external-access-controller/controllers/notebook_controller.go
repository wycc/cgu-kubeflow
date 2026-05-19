package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	notebookv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1beta1"
)

// NotebookReconciler reconciles a Notebook object
type NotebookReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubeflow.org,resources=notebooks,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kubeflow.org,resources=notebooks/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile 監聽 Notebook 資源的變化，根據是否帶有 SSHServiceLabel 決定是否建立或刪除
// SSH NodePort Service，並在 Service 建立後將指派的 NodePort 回寫至 Notebook Annotation。
func (r *NotebookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 取得 Notebook 實例
	notebook := &notebookv1.Notebook{}
	if err := r.Get(ctx, req.NamespacedName, notebook); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// Notebook 已不存在（可能已被刪除），忽略
		return ctrl.Result{}, nil
	}

	// 若 Notebook 正在刪除中，由 OwnerReference 機制自動清理 Service
	if !notebook.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// 判斷是否啟用 SSH Service（依 Label 決定）
	sshEnabled := notebook.Labels[SSHServiceLabel] == "true"

	// 查詢對應的 SSH Service 是否已存在
	serviceName := fmt.Sprintf("%s-ssh-service", notebook.Name)
	service := &corev1.Service{}
	getErr := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: notebook.Namespace}, service)

	if !sshEnabled {
		// 未啟用 SSH：若 Service 存在則刪除，並清除回寫的 NodePort Annotation
		if getErr == nil {
			logger.Info("SSH label 未設定，刪除 SSH Service", "Notebook", notebook.Name, "Service", serviceName)
			if err := r.Delete(ctx, service); err != nil {
				logger.Error(err, "刪除 SSH Service 失敗")
				return ctrl.Result{}, err
			}
		}
		// 清除 Notebook 上的 NodePort Annotation
		if err := r.clearSSHNodePortAnnotation(ctx, notebook); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// 已啟用 SSH：確保 Service 存在
	if getErr != nil {
		if client.IgnoreNotFound(getErr) == nil {
			// Service 不存在，建立新的 SSH Service
			newService := r.newSSHServiceForNotebook(notebook)
			logger.Info("建立 SSH Service", "Service.Namespace", newService.Namespace, "Service.Name", newService.Name)
			if err := r.Create(ctx, newService); err != nil {
				logger.Error(err, "建立 SSH Service 失敗")
				return ctrl.Result{}, err
			}
			// Service 剛建立，NodePort 尚未指派，重新入列等待
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error(getErr, "查詢 SSH Service 失敗")
		return ctrl.Result{}, getErr
	}

	// Service 已存在，將 NodePort 回寫至 Notebook Annotation
	if len(service.Spec.Ports) > 0 && service.Spec.Ports[0].NodePort > 0 {
		nodePort := service.Spec.Ports[0].NodePort
		if err := r.setSSHNodePortAnnotation(ctx, notebook, nodePort); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// newSSHServiceForNotebook 建立一個以 NodePort 類型暴露 SSH 的 Service。
// 固定使用 SSHServicePort（2222）作為 Service Port，
// SSHContainerPort（22）作為容器目標 Port。
func (r *NotebookReconciler) newSSHServiceForNotebook(notebook *notebookv1.Notebook) *corev1.Service {
	labels := map[string]string{
		"app":           notebook.Name,
		"notebook-name": notebook.Name,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ssh-service", notebook.Name),
			Namespace: notebook.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "tcp-ssh",
					Protocol:   corev1.ProtocolTCP,
					Port:       SSHServicePort,
					TargetPort: intstr.FromInt(int(SSHContainerPort)),
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}

	// 設定 Notebook 為 Service 的擁有者，確保 Notebook 刪除時 Service 自動回收
	ctrl.SetControllerReference(notebook, service, r.Scheme)
	return service
}

// setSSHNodePortAnnotation 將 NodePort 值回寫至 Notebook 的 Annotation，
// 僅在數值有變更時才呼叫 Update，避免不必要的 API 請求。
func (r *NotebookReconciler) setSSHNodePortAnnotation(ctx context.Context, notebook *notebookv1.Notebook, nodePort int32) error {
	annotations := notebook.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	newVal := fmt.Sprintf("%d", nodePort)
	if annotations[SSHNodePortAnnotation] == newVal {
		// 值未改變，無需更新
		return nil
	}

	annotations[SSHNodePortAnnotation] = newVal
	notebook.SetAnnotations(annotations)

	logger := log.Log.WithValues("Notebook", notebook.Name, "NodePort", newVal)
	logger.Info("回寫 SSH NodePort Annotation 至 Notebook")
	return r.Update(ctx, notebook)
}

// clearSSHNodePortAnnotation 從 Notebook 的 Annotation 中移除 SSH NodePort 記錄。
func (r *NotebookReconciler) clearSSHNodePortAnnotation(ctx context.Context, notebook *notebookv1.Notebook) error {
	annotations := notebook.GetAnnotations()
	if annotations == nil {
		return nil
	}
	if _, exists := annotations[SSHNodePortAnnotation]; !exists {
		// Annotation 本就不存在，無需操作
		return nil
	}

	delete(annotations, SSHNodePortAnnotation)
	notebook.SetAnnotations(annotations)

	log.Log.WithValues("Notebook", notebook.Name).Info("清除 SSH NodePort Annotation")
	return r.Update(ctx, notebook)
}

// SetupWithManager 將 NotebookReconciler 註冊至 Controller Manager。
// 監聽 Notebook 資源的異動，以及由 Notebook 擁有的 Service 的異動。
// 詳見 OperatorPattern.md。
func (r *NotebookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&notebookv1.Notebook{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
