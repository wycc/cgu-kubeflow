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

	notebookv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1beta1" // 引入 Kubeflow Notebook API
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify Reconcile to be able to reconcile your custom objects.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *NotebookReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Fetch the Notebook instance
	notebook := &notebookv1.Notebook{}
	if err := r.Get(ctx, req.NamespacedName, notebook); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		// Notebook not found, could be deleted
		return ctrl.Result{}, nil
	}

	// Check if the Notebook is being deleted
	if !notebook.ObjectMeta.DeletionTimestamp.IsZero() {
		// Notebook is being deleted, clean up associated resources if any
		// For now, we rely on owner reference for Service deletion
		return ctrl.Result{}, nil
	}

	// Extract the second port from the Notebook's containers
	sshPort := int32(0)
	if len(notebook.Spec.Template.Spec.Containers) > 0 && len(notebook.Spec.Template.Spec.Containers[0].Ports) > 1 {
		sshPort = notebook.Spec.Template.Spec.Containers[0].Ports[1].ContainerPort
	}

	// Define the desired SSH Service
	serviceName := fmt.Sprintf("%s-ssh-service", notebook.Name)
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: notebook.Namespace}, service)

	if sshPort == 0 {
		// If SSH port is not defined, ensure the service is deleted
		if err == nil {
			// Service exists, so we need to delete it
			log.Log.Info("SSH port is not defined for this Notebook, deleting existing Service", "Notebook", notebook.Name)
			if err := r.Delete(ctx, service); err != nil {
				log.Log.Error(err, "Failed to delete SSH Service")
				return ctrl.Result{}, err
			}
		}
		// If service doesn't exist, we are in the desired state.
		return ctrl.Result{}, nil
	}

	// SSH port is defined, proceed with Service creation/update
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Service does not exist, create it
			service = r.newSSHServiceForNotebook(notebook, sshPort)
			log.Log.Info("Creating a new SSH Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			if err := r.Create(ctx, service); err != nil {
				log.Log.Error(err, "Failed to create new SSH Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		log.Log.Error(err, "Failed to get SSH Service")
		return ctrl.Result{}, err
	}

	// Service already exists, check if it needs update
	needsUpdate := false
	if len(service.Spec.Ports) > 0 {
		if service.Spec.Ports[0].Port != 2222 {
			service.Spec.Ports[0].Port = 2222
			service.Spec.Ports[0].TargetPort = intstr.FromInt(int(sshPort))
			needsUpdate = true
		}
	}

	if needsUpdate {
		log.Log.Info("Updating existing SSH Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
		if err := r.Update(ctx, service); err != nil {
			log.Log.Error(err, "Failed to update existing SSH Service", "Service.Namespace", service.Namespace, "Service.Name", service.Name)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// newSSHServiceForNotebook creates a new Service for a Notebook resource.
func (r *NotebookReconciler) newSSHServiceForNotebook(notebook *notebookv1.Notebook, sshPort int32) *corev1.Service {
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
					Port:       sshPort,
					TargetPort: intstr.FromInt(int(sshPort)),
				},
			},
			Type: corev1.ServiceTypeNodePort,
		},
	}
	// Set the Notebook instance as the owner and controller
	ctrl.SetControllerReference(notebook, service, r.Scheme)
	return service
}

// SetupWithManager sets up the controller with the Manager.
// Please look at OperatorPattern.md for the defails.
func (r *NotebookReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&notebookv1.Notebook{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
