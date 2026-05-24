package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

const ExternalAccessAnnotation = "kflow.cgu.com.tw/external-access"
const ExternalNodePortAnnotation = "kflow.cgu.com.tw/external-nodeports"

// ExternalAccessReconciler reconciles a Notebook object to manage external access services
type ExternalAccessReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=kubeflow.org,resources=notebooks,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *ExternalAccessReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	notebook := &notebookv1.Notebook{}
	if err := r.Get(ctx, req.NamespacedName, notebook); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if !notebook.ObjectMeta.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	externalPortsStr := notebook.Annotations[ExternalAccessAnnotation]
	serviceName := fmt.Sprintf("%s-ext-access", notebook.Name)
	service := &corev1.Service{}
	getErr := r.Get(ctx, types.NamespacedName{Name: serviceName, Namespace: notebook.Namespace}, service)

	if externalPortsStr == "" {
		if getErr == nil {
			logger.Info("External access annotation removed, deleting Service", "Notebook", notebook.Name, "Service", serviceName)
			if err := r.Delete(ctx, service); err != nil {
				return ctrl.Result{}, err
			}
		}
		if err := r.clearExternalNodePortAnnotation(ctx, notebook); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	ports := r.parsePorts(externalPortsStr)
	if len(ports) == 0 {
		return ctrl.Result{}, nil // Invalid ports
	}

	if getErr != nil {
		if client.IgnoreNotFound(getErr) == nil {
			newService := r.newServiceForNotebook(notebook, ports)
			logger.Info("Creating External Access Service", "Service", newService.Name)
			if err := r.Create(ctx, newService); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, getErr
	}

	// Update service if needed
	if r.needsUpdate(service, ports) {
		// Only update the ports to avoid overwriting dynamically assigned fields (like NodePort or ClusterIP)
		service.Spec.Ports = r.mergePorts(service.Spec.Ports, ports)
		if err := r.Update(ctx, service); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Write back NodePorts
	if err := r.setExternalNodePortAnnotation(ctx, notebook, service); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ExternalAccessReconciler) parsePorts(portStr string) []int32 {
	var ports []int32
	parts := strings.Split(portStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		port, err := strconv.Atoi(p)
		if err == nil && port > 0 && port <= 65535 {
			ports = append(ports, int32(port))
		}
	}
	return ports
}

func (r *ExternalAccessReconciler) generateServicePorts(ports []int32) []corev1.ServicePort {
	var svcPorts []corev1.ServicePort
	for _, p := range ports {
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       fmt.Sprintf("tcp-%d", p),
			Protocol:   corev1.ProtocolTCP,
			Port:       p,
			TargetPort: intstr.FromInt(int(p)),
		})
	}
	return svcPorts
}

func (r *ExternalAccessReconciler) mergePorts(existingPorts []corev1.ServicePort, desiredPorts []int32) []corev1.ServicePort {
	var newPorts []corev1.ServicePort
	existingMap := make(map[int32]corev1.ServicePort)
	for _, p := range existingPorts {
		existingMap[p.Port] = p
	}

	for _, p := range desiredPorts {
		if existing, ok := existingMap[p]; ok {
			newPorts = append(newPorts, existing)
		} else {
			newPorts = append(newPorts, corev1.ServicePort{
				Name:       fmt.Sprintf("tcp-%d", p),
				Protocol:   corev1.ProtocolTCP,
				Port:       p,
				TargetPort: intstr.FromInt(int(p)),
			})
		}
	}
	return newPorts
}

func (r *ExternalAccessReconciler) needsUpdate(svc *corev1.Service, ports []int32) bool {
	if len(svc.Spec.Ports) != len(ports) {
		return true
	}
	portMap := make(map[int32]bool)
	for _, p := range ports {
		portMap[p] = true
	}
	for _, sp := range svc.Spec.Ports {
		if !portMap[sp.Port] {
			return true
		}
	}
	return false
}

func (r *ExternalAccessReconciler) newServiceForNotebook(notebook *notebookv1.Notebook, ports []int32) *corev1.Service {
	labels := map[string]string{
		"app":           notebook.Name,
		"notebook-name": notebook.Name,
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ext-access", notebook.Name),
			Namespace: notebook.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    r.generateServicePorts(ports),
			Type:     corev1.ServiceTypeNodePort,
		},
	}

	ctrl.SetControllerReference(notebook, service, r.Scheme)
	return service
}

func (r *ExternalAccessReconciler) setExternalNodePortAnnotation(ctx context.Context, notebook *notebookv1.Notebook, svc *corev1.Service) error {
	var nodePorts []string
	for _, p := range svc.Spec.Ports {
		if p.NodePort > 0 {
			nodePorts = append(nodePorts, fmt.Sprintf("%d", p.NodePort))
		}
	}
	if len(nodePorts) == 0 {
		return nil
	}

	newVal := strings.Join(nodePorts, ",")
	annotations := notebook.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	if annotations[ExternalNodePortAnnotation] == newVal {
		return nil
	}

	annotations[ExternalNodePortAnnotation] = newVal
	notebook.SetAnnotations(annotations)

	logger := log.Log.WithValues("Notebook", notebook.Name, "ExternalNodePorts", newVal)
	logger.Info("Updated Notebook external-nodeports annotation")
	return r.Update(ctx, notebook)
}

func (r *ExternalAccessReconciler) clearExternalNodePortAnnotation(ctx context.Context, notebook *notebookv1.Notebook) error {
	annotations := notebook.GetAnnotations()
	if annotations == nil {
		return nil
	}
	if _, exists := annotations[ExternalNodePortAnnotation]; !exists {
		return nil
	}

	delete(annotations, ExternalNodePortAnnotation)
	notebook.SetAnnotations(annotations)

	logger := log.Log.WithValues("Notebook", notebook.Name)
	logger.Info("Cleared Notebook external-nodeports annotation")
	return r.Update(ctx, notebook)
}

func (r *ExternalAccessReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("external-access").
		For(&notebookv1.Notebook{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
