package controllers

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StatefulSetReconciler reconciles a StatefulSet object
type StatefulSetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StatefulSetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the StatefulSet instance
	statefulSet := &appsv1.StatefulSet{}
	if err := r.Get(ctx, req.NamespacedName, statefulSet); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// StatefulSet not found, could be deleted
		return ctrl.Result{}, nil
	}

	// Check if the StatefulSet is being deleted
	if !statefulSet.ObjectMeta.DeletionTimestamp.IsZero() {
		// StatefulSet is being deleted, nothing to do
		return ctrl.Result{}, nil
	}

	// Check if this StatefulSet is created by notebook
	if !r.isNotebookStatefulSet(statefulSet) {
		// Not a notebook StatefulSet, skip
		return ctrl.Result{}, nil
	}

	// Check if the StatefulSet has a second port defined
	if !r.hasSecondPort(statefulSet) {
		// No second port defined, skip
		return ctrl.Result{}, nil
	}

	// Check if the Istio annotation is already present
	if r.hasIstioAnnotation(statefulSet) {
		// Annotation already exists, nothing to do
		return ctrl.Result{}, nil
	}

	// Add the Istio annotation
	if err := r.addIstioAnnotation(ctx, statefulSet); err != nil {
		logger.Error(err, "Failed to add Istio annotation to StatefulSet", "StatefulSet", statefulSet.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Successfully added Istio annotation to StatefulSet", "StatefulSet", statefulSet.Name)
	return ctrl.Result{}, nil
}

// isNotebookStatefulSet checks if the StatefulSet is created by a notebook
func (r *StatefulSetReconciler) isNotebookStatefulSet(statefulSet *appsv1.StatefulSet) bool {
	// Check for notebook-related labels or owner references
	labels := statefulSet.GetLabels()
	if labels == nil {
		return false
	}

	// Check for common notebook labels
	if _, exists := labels["notebook-name"]; exists {
		return true
	}
	if _, exists := labels["app.kubernetes.io/component"]; exists && labels["app.kubernetes.io/component"] == "notebook" {
		return true
	}
	if _, exists := labels["kubeflow-resource-type"]; exists && labels["kubeflow-resource-type"] == "notebook" {
		return true
	}

	// Check owner references for notebook controller
	for _, ownerRef := range statefulSet.GetOwnerReferences() {
		if ownerRef.Kind == "Notebook" && ownerRef.APIVersion == "kubeflow.org/v1beta1" {
			return true
		}
	}

	return false
}

// hasSecondPort checks if the StatefulSet has a second port defined in its containers
func (r *StatefulSetReconciler) hasSecondPort(statefulSet *appsv1.StatefulSet) bool {
	if statefulSet.Spec.Template.Spec.Containers == nil || len(statefulSet.Spec.Template.Spec.Containers) == 0 {
		return false
	}

	// Check the first container for ports
	container := statefulSet.Spec.Template.Spec.Containers[0]
	if container.Ports == nil || len(container.Ports) < 2 {
		return false
	}

	return true
}

// hasIstioAnnotation checks if the StatefulSet already has the Istio annotation
func (r *StatefulSetReconciler) hasIstioAnnotation(statefulSet *appsv1.StatefulSet) bool {
	annotations := statefulSet.Spec.Template.GetAnnotations()
	if annotations == nil {
		return false
	}

	_, exists := annotations["traffic.sidecar.istio.io/excludeInboundPorts"]
	return exists
}

// addIstioAnnotation adds the Istio annotation to the StatefulSet
func (r *StatefulSetReconciler) addIstioAnnotation(ctx context.Context, statefulSet *appsv1.StatefulSet) error {
	// Get current annotations or create new map
	annotations := statefulSet.Spec.Template.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Add the Istio annotation
	annotations["traffic.sidecar.istio.io/excludeInboundPorts"] = "22"

	// Update the StatefulSet template annotations
	statefulSet.Spec.Template.SetAnnotations(annotations)

	// Update the StatefulSet
	return r.Update(ctx, statefulSet)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StatefulSetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1.StatefulSet{}).
		Complete(r)
}
