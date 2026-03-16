package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	notebookv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1beta1"
)

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch

// Reconcile 監聽 StatefulSet 的變化，針對隸屬於 Kubeflow Notebook 的 StatefulSet，
// 依父 Notebook 是否帶有 SSHServiceLabel 來新增或移除 Istio 流量排除 Annotation。
func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 取得 StatefulSet 實例
	statefulSet := &appsv1.StatefulSet{}
	if err := r.Get(ctx, req.NamespacedName, statefulSet); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// StatefulSet 已不存在，忽略
		return ctrl.Result{}, nil
	}

	// 若 StatefulSet 正在刪除中，無需處理
	if !statefulSet.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	// 確認此 StatefulSet 是否由 Notebook 建立
	if !r.isNotebookStatefulSet(statefulSet) {
		return ctrl.Result{}, nil
	}

	// 查詢父 Notebook 是否啟用了 SSH Service
	sshEnabled, err := r.isSSHEnabledOnParentNotebook(ctx, statefulSet)
	if err != nil {
		logger.Error(err, "查詢父 Notebook SSH label 失敗", "StatefulSet", statefulSet.Name)
		return ctrl.Result{}, err
	}

	if !sshEnabled {
		// SSH 未啟用：若 Istio 排除 Annotation 存在則移除
		if r.hasIstioAnnotation(statefulSet) {
			if err := r.removeIstioAnnotation(ctx, statefulSet); err != nil {
				logger.Error(err, "移除 Istio 排除 Annotation 失敗", "StatefulSet", statefulSet.Name)
				return ctrl.Result{}, err
			}
			logger.Info("已從 StatefulSet 移除 Istio 排除 Annotation", "StatefulSet", statefulSet.Name)
		}
		return ctrl.Result{}, nil
	}

	// SSH 已啟用：確保 Istio 排除 Annotation 存在
	if r.hasIstioAnnotation(statefulSet) {
		// Annotation 已存在，無需操作
		return ctrl.Result{}, nil
	}

	if err := r.addIstioAnnotation(ctx, statefulSet); err != nil {
		logger.Error(err, "新增 Istio 排除 Annotation 失敗", "StatefulSet", statefulSet.Name)
		return ctrl.Result{}, err
	}

	logger.Info("已為 StatefulSet 新增 Istio 排除 Annotation", "StatefulSet", statefulSet.Name)
	return ctrl.Result{}, nil
}

// isNotebookStatefulSet 判斷此 StatefulSet 是否由 Kubeflow Notebook 建立。
// 符合以下任一條件即視為 Notebook 的 StatefulSet：
//   - Labels 含有 "notebook-name"
//   - Labels 含有 app.kubernetes.io/component=notebook
//   - Labels 含有 kubeflow-resource-type=notebook
//   - OwnerReferences 中有 Kind=Notebook 且 APIVersion=kubeflow.org/v1beta1
func (r *StatefulSetReconciler) isNotebookStatefulSet(statefulSet *appsv1.StatefulSet) bool {
	labels := statefulSet.GetLabels()
	if labels != nil {
		if _, exists := labels["notebook-name"]; exists {
			return true
		}
		if labels["app.kubernetes.io/component"] == "notebook" {
			return true
		}
		if labels["kubeflow-resource-type"] == "notebook" {
			return true
		}
	}

	for _, ownerRef := range statefulSet.GetOwnerReferences() {
		if ownerRef.Kind == "Notebook" && ownerRef.APIVersion == "kubeflow.org/v1beta1" {
			return true
		}
	}

	return false
}

// isSSHEnabledOnParentNotebook 透過 OwnerReference 找到父 Notebook，
// 並檢查其是否帶有 SSHServiceLabel="true"。
// 若找不到父 Notebook 或 Notebook 不存在，回傳 false。
func (r *StatefulSetReconciler) isSSHEnabledOnParentNotebook(ctx context.Context, statefulSet *appsv1.StatefulSet) (bool, error) {
	for _, ownerRef := range statefulSet.GetOwnerReferences() {
		if ownerRef.Kind != "Notebook" || ownerRef.APIVersion != "kubeflow.org/v1beta1" {
			continue
		}

		notebook := &notebookv1.Notebook{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      ownerRef.Name,
			Namespace: statefulSet.Namespace,
		}, notebook); err != nil {
			if client.IgnoreNotFound(err) == nil {
				// Notebook 已不存在，視為未啟用
				return false, nil
			}
			return false, err
		}

		return notebook.Labels[SSHServiceLabel] == "true", nil
	}

	// 找不到 Notebook OwnerReference，視為未啟用
	return false, nil
}

// hasIstioAnnotation 檢查 StatefulSet 的 Pod Template 是否已有 Istio 排除 Annotation。
func (r *StatefulSetReconciler) hasIstioAnnotation(statefulSet *appsv1.StatefulSet) bool {
	annotations := statefulSet.Spec.Template.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, exists := annotations[IstioExcludePortAnnotation]
	return exists
}

// addIstioAnnotation 在 StatefulSet 的 Pod Template Annotations 中加入 Istio 排除規則，
// 讓 Istio Sidecar 略過對 SSH 入站流量（Port 22）的攔截。
func (r *StatefulSetReconciler) addIstioAnnotation(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	annotations := statefulSet.Spec.Template.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[IstioExcludePortAnnotation] = IstioExcludePortValue
	statefulSet.Spec.Template.SetAnnotations(annotations)
	return r.Update(ctx, statefulSet)
}

// removeIstioAnnotation 從 StatefulSet 的 Pod Template Annotations 中移除 Istio 排除規則。
func (r *StatefulSetReconciler) removeIstioAnnotation(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	annotations := statefulSet.Spec.Template.GetAnnotations()
	if annotations == nil {
		return nil
	}
	delete(annotations, IstioExcludePortAnnotation)
	statefulSet.Spec.Template.SetAnnotations(annotations)
	return r.Update(ctx, statefulSet)
}

// SetupWithManager 將 StatefulSetReconciler 註冊至 Controller Manager。
func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}
