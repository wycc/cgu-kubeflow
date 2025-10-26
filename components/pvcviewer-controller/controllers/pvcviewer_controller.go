/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubefloworgv1alpha1 "github.com/kubeflow/kubeflow/components/pvc-viewer/api/v1alpha1"
)

// PVCViewerReconciler reconciles a PVCViewer object
type PVCViewerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	// We use a resource prefix so that the names of generated resources like deployments are unique
	resourcePrefix = "pvcviewer-"

	nameLabelKey     = "app.kubernetes.io/name"
	instanceLabelKey = "app.kubernetes.io/instance"
	partOfLabelKey   = "app.kubernetes.io/part-of"
	partOfLabelValue = "pvc-viewer"

	istioGatewayEnvKey  = "ISTIO_GATEWAY"
	defaultIstioGateway = "kubeflow/kubeflow-gateway"

	// Finalizer for public share resources cleanup
	publicShareFinalizer = "pvcviewer.kubeflow.org/public-share-cleanup"

	// Labels for tracking istio-system resources
	pvcviewerNameLabelKey      = "pvcviewer.kubeflow.org/name"
	pvcviewerNamespaceLabelKey = "pvcviewer.kubeflow.org/namespace"
)

var (
	servicePort = int32(80)

	virtualServiceTemplate = &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1alpha3",
			"kind":       "VirtualService",
		},
	}
)

// Default permissions for the PVCViewer
// +kubebuilder:rbac:groups=kubeflow.org,resources=pvcviewers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubeflow.org,resources=pvcviewers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubeflow.org,resources=pvcviewers/finalizers,verbs=update

// Add permissions to create child resources
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update

// Add permissions to read external resources
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch

// Public share resources - EnvoyFilter and AuthorizationPolicy
// +kubebuilder:rbac:groups=networking.istio.io,resources=envoyfilters,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies,verbs=get;list;watch;create;update;delete

// SetupWithManager sets up the controller with the Manager.
func (r *PVCViewerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubefloworgv1alpha1.PVCViewer{}).
		// This controller manages, i.e. creates these kinds for a PVCViewer
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(virtualServiceTemplate).
		Complete(r)
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PVCViewerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	instance := &kubefloworgv1alpha1.PVCViewer{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		// Created objects are automatically garbage collected if parent is deleted
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if !instance.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is being deleted
		log.Info("PVCViewer is being deleted")

		// Handle finalizer cleanup for istio-system resources
		if containsString(instance.Finalizers, publicShareFinalizer) {
			log.Info("Cleaning up public share resources in istio-system")
			if err := r.cleanupIstioSystemResources(ctx, log, instance); err != nil {
				log.Error(err, "Failed to cleanup istio-system resources")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			instance.Finalizers = removeString(instance.Finalizers, publicShareFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Keep on reconciling status until all finalizers are removed
		if err := r.reconcileStatus(ctx, log, instance.Name, instance.Namespace); err != nil {
			log.Error(err, "Error while reconciling status")
			return ctrl.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	// Ensure finalizer is present when Networking is configured
	if instance.Spec.Networking != (kubefloworgv1alpha1.Networking{}) {
		if !containsString(instance.Finalizers, publicShareFinalizer) {
			log.Info("Adding public share finalizer")
			instance.Finalizers = append(instance.Finalizers, publicShareFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				return ctrl.Result{}, err
			}
			// Requeue to continue processing
			return ctrl.Result{Requeue: true}, nil
		}
	}

	commonLabels := map[string]string{
		nameLabelKey:     instance.Name,
		instanceLabelKey: resourcePrefix + instance.Name,
		partOfLabelKey:   partOfLabelValue,
	}

	if err := r.reconcileDeployment(ctx, log, instance, commonLabels); err != nil {
		log.Error(err, "Error while reconciling deployment")
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, log, instance, commonLabels); err != nil {
		log.Error(err, "Error while reconciling service")
		return ctrl.Result{}, err
	}

	if err := r.reconcileVirtualService(ctx, log, instance, commonLabels); err != nil {
		log.Error(err, "Error while reconciling virtual service")
		return ctrl.Result{}, err
	}

	// Reconcile public share resources
	if err := r.reconcilePublicShareResources(ctx, log, instance, commonLabels); err != nil {
		log.Error(err, "Error while reconciling public share resources")
		return ctrl.Result{}, err
	}

	if err := r.reconcileStatus(ctx, log, instance.Name, instance.Namespace); err != nil {
		log.Error(err, "Error while reconciling status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Creates or updates the deployment as defined by the viewer's podSpec
func (r *PVCViewerReconciler) reconcileDeployment(ctx context.Context, log logr.Logger, viewer *kubefloworgv1alpha1.PVCViewer, commonLabels map[string]string) error {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourcePrefix + viewer.Name,
			Namespace: viewer.Namespace,
			Labels:    commonLabels,
		},
	}
	createDeployment := false
	if err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, deployment); err != nil {
		if !apierrs.IsNotFound(err) {
			return err
		}
		createDeployment = true
	}

	var (
		// Do not change affinity or rwoClaims by default
		affinity = deployment.Spec.Template.Spec.Affinity
		// Affinity is only to be set when rwo scheduling is enabled and the deployment is to be newly created
		determineAffinity = viewer.Spec.RWOScheduling && createDeployment
	)

	if determineAffinity {
		if newAffinity, err := r.generateAffinity(ctx, log, viewer); err != nil {
			return err
		} else if newAffinity != nil {
			// Only set the affinity if it is not nil - we wouldn't win anything by restarting without affinity
			affinity = newAffinity
		}
	}

	deployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: commonLabels,
	}
	deployment.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: commonLabels,
		},
		Spec: viewer.Spec.PodSpec,
	}
	// We're using a recreate strategy to ensure that the pod is restarted when the affinity change.
	// Otherwise, we could be mounting the same PVC to multiple pods, preventing the pod from starting.
	deployment.Spec.Strategy = appsv1.DeploymentStrategy{
		Type: appsv1.RecreateDeploymentStrategyType,
	}
	deployment.Spec.Template.Spec.Affinity = affinity

	if err := ctrl.SetControllerReference(viewer, deployment, r.Scheme); err != nil {
		return err
	}

	if createDeployment {
		log.Info("Creating Deployment")
		return r.Create(ctx, deployment)
	}
	log.Info("Updating Deployment")
	return r.Update(ctx, deployment)
}

// Creates or updates the service as defined by the viewer's service
func (r *PVCViewerReconciler) reconcileService(ctx context.Context, log logr.Logger, viewer *kubefloworgv1alpha1.PVCViewer, commonLabels map[string]string) error {
	if viewer.Spec.Networking == (kubefloworgv1alpha1.Networking{}) {
		return nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourcePrefix + viewer.Name,
			Namespace: viewer.Namespace,
			Labels:    commonLabels,
		},
	}
	createService := false
	if err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, service); err != nil {
		if !apierrs.IsNotFound(err) {
			return err
		}
		createService = true
	}

	service.Spec.Type = "ClusterIP"
	service.Spec.Selector = commonLabels
	service.Spec.Ports = []corev1.ServicePort{
		{
			Name:       "http",
			Port:       servicePort,
			TargetPort: viewer.Spec.Networking.TargetPort,
		},
	}

	if err := ctrl.SetControllerReference(viewer, service, r.Scheme); err != nil {
		return err
	}

	if createService {
		log.Info("Creating Service")
		return r.Create(ctx, service)
	}
	log.Info("Updating Service")
	return r.Update(ctx, service)
}

func (r *PVCViewerReconciler) reconcileVirtualService(ctx context.Context, log logr.Logger, viewer *kubefloworgv1alpha1.PVCViewer, commonLabels map[string]string) error {
	if viewer.Spec.Networking == (kubefloworgv1alpha1.Networking{}) {
		return nil
	}

	virtualService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1alpha3",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      resourcePrefix + viewer.Name,
				"namespace": viewer.Namespace,
				"labels":    commonLabels,
			},
		},
	}
	createVirtualService := false
	if err := r.Get(ctx, types.NamespacedName{Name: virtualService.GetName(), Namespace: virtualService.GetNamespace()}, virtualService); err != nil {
		if !apierrs.IsNotFound(err) {
			return err
		}
		createVirtualService = true
	}

	prefix := fmt.Sprintf("%s/%s/%s/", viewer.Spec.Networking.BasePrefix, viewer.Namespace, viewer.Name)
	rewrite := prefix
	if viewer.Spec.Networking.Rewrite != "" {
		rewrite = viewer.Spec.Networking.Rewrite
	}
	service := fmt.Sprintf("%s%s.%s.svc.cluster.local", resourcePrefix, viewer.Name, viewer.Namespace)
	var timeout *string = nil
	if viewer.Spec.Networking.Timeout != "" {
		timeout = &viewer.Spec.Networking.Timeout
	}

	// Get the istio gateway from the environment variable or use the default
	istioGateway := os.Getenv(istioGatewayEnvKey)
	if istioGateway == "" {
		istioGateway = defaultIstioGateway
	}

	virtualService.Object["spec"] = map[string]interface{}{
		"hosts": []string{"*"},
		"gateways": []string{
			istioGateway,
		},
		"http": []interface{}{
			map[string]interface{}{
				"match": []interface{}{
					map[string]interface{}{
						"uri": map[string]interface{}{
							"prefix": prefix,
						},
					},
				},
				"rewrite": map[string]interface{}{
					"uri": rewrite,
				},
				"route": []interface{}{
					map[string]interface{}{
						"destination": map[string]interface{}{
							"host": service,
							"port": map[string]interface{}{
								"number": int64(servicePort),
							},
						},
					},
				},
				"timeout": timeout,
			},
		},
	}

	if err := ctrl.SetControllerReference(viewer, virtualService, r.Scheme); err != nil {
		return err
	}

	if createVirtualService {
		log.Info("Creating Virtual Service")
		return r.Create(ctx, virtualService)
	}
	log.Info("Updating Virtual Service")
	return r.Update(ctx, virtualService)
}

// Computes and updates the status of the PVCViewer
func (r *PVCViewerReconciler) reconcileStatus(ctx context.Context, log logr.Logger, viewerName string, viewerNamespace string) error {
	viewer := &kubefloworgv1alpha1.PVCViewer{}
	if err := r.Get(ctx, types.NamespacedName{Name: viewerName, Namespace: viewerNamespace}, viewer); err != nil {
		return err
	}

	if viewer.Spec.Networking != (kubefloworgv1alpha1.Networking{}) {
		url := fmt.Sprintf("%s/%s/%s/", viewer.Spec.Networking.BasePrefix, viewer.Namespace, viewer.Name)
		viewer.Status.URL = &url
	} else {
		viewer.Status.URL = nil
	}

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Name: resourcePrefix + viewer.Name, Namespace: viewer.Namespace}, deployment); err != nil {
		log.Info("Could not find Deployment for status update")
		viewer.Status.Ready = false
	} else {
		viewer.Status.Ready = *deployment.Spec.Replicas == deployment.Status.ReadyReplicas
		// Append the latest condition, if it is not already in the list
		if len(deployment.Status.Conditions) > 0 {
			clen := len(viewer.Status.Conditions)
			if clen == 0 || viewer.Status.Conditions[clen-1] != deployment.Status.Conditions[0] {
				viewer.Status.Conditions = append(viewer.Status.Conditions, deployment.Status.Conditions[0])
			}
		}
	}

	log.Info("Updating status")
	return r.Client.Status().Update(ctx, viewer)
}

// Generates the affinity to be used for the deployment
// In case no affinity should be used (e.g. RWOScheduling is disabled) or updated, nil is returned
func (r *PVCViewerReconciler) generateAffinity(ctx context.Context, log logr.Logger, viewer *kubefloworgv1alpha1.PVCViewer) (*corev1.Affinity, error) {
	// Check if the viewer's PVC is RWO access mode
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: viewer.Spec.PVC, Namespace: viewer.Namespace}, pvc); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Omitting Affinity: PVC not found")
			// Should we return an error here or suppress it and let the Deployment fail?
			// Latter might be better and more visible to the user
			return nil, nil
		}
		return nil, err
	}

	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		log.Info("Omitting Affinity: PVC is not RWO")
		return nil, nil
	}

	// Get all pods in namespace and filter by RWO PVCs
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(viewer.Namespace)); err != nil {
		return nil, err
	}
	var nodeName *string
	for _, pod := range podList.Items {
		// Skip pods this controller created
		if partOf, ok := pod.Labels[partOfLabelKey]; ok && partOf == partOfLabelValue {
			continue
		}
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName != "" {
				if volume.PersistentVolumeClaim.ClaimName == pvc.Name {
					if nodeName != nil {
						// Rather than throwing an error, we just omit the affinity, leaving the current deployment's affinity unchanged
						log.Info("Omitting Affinity: Viewer references RWO volumes on multiple nodes",
							"nodes", []string{*nodeName, pod.Spec.NodeName})
						return nil, nil
					}
					if pod.Spec.NodeName == "" {
						log.Info("Omitting Affinity: Viewer references RWO volume on pod without nodeName")
						return nil, nil
					}
					nodeName = &pod.Spec.NodeName
				}
			}
		}
	}

	if nodeName == nil {
		log.Info("Omitting Affinity: PVC not used by other Pods")
		return nil, nil
	}

	// Generate Affinity using the node name
	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
				{
					Weight: 100,
					Preference: corev1.NodeSelectorTerm{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "kubernetes.io/hostname",
								Operator: "In",
								Values:   []string{*nodeName},
							},
						},
					},
				},
			},
		},
	}
	return affinity, nil
}

// reconcilePublicShareResources creates and manages public share resources when Networking is configured
func (r *PVCViewerReconciler) reconcilePublicShareResources(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
	commonLabels map[string]string,
) error {
	// Only create public share resources when Networking is configured
	if viewer.Spec.Networking == (kubefloworgv1alpha1.Networking{}) {
		log.Info("Skipping public share resources: Networking not configured")
		return nil
	}

	// Reconcile each resource
	if err := r.reconcileShareVirtualService(ctx, log, viewer, commonLabels); err != nil {
		return fmt.Errorf("failed to reconcile share VirtualService: %w", err)
	}

	if err := r.reconcileShareEnvoyFilter(ctx, log, viewer); err != nil {
		return fmt.Errorf("failed to reconcile share EnvoyFilter: %w", err)
	}

	if err := r.reconcileShareAuthPolicyGateway(ctx, log, viewer); err != nil {
		return fmt.Errorf("failed to reconcile share AuthorizationPolicy (Gateway): %w", err)
	}

	if err := r.reconcileShareAuthPolicyBackend(ctx, log, viewer, commonLabels); err != nil {
		return fmt.Errorf("failed to reconcile share AuthorizationPolicy (Backend): %w", err)
	}

	return nil
}

// reconcileShareVirtualService creates or updates the VirtualService for public share access
func (r *PVCViewerReconciler) reconcileShareVirtualService(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
	commonLabels map[string]string,
) error {
	vsName := fmt.Sprintf("pvcviewer-share-%s", viewer.Name)
	routeName := fmt.Sprintf("pvcviewer-%s-%s-share-public", viewer.Namespace, viewer.Name)
	sharePath := fmt.Sprintf("%s/%s/%s/share/",
		viewer.Spec.Networking.BasePrefix,
		viewer.Namespace,
		viewer.Name)

	// Get the istio gateway from the environment variable or use the default
	istioGateway := os.Getenv(istioGatewayEnvKey)
	if istioGateway == "" {
		istioGateway = defaultIstioGateway
	}

	vs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      vsName,
				"namespace": viewer.Namespace,
				"labels":    commonLabels,
			},
			"spec": map[string]interface{}{
				"hosts":    []string{"*"},
				"gateways": []string{istioGateway},
				"http": []interface{}{
					map[string]interface{}{
						"name": routeName,
						"match": []interface{}{
							map[string]interface{}{
								"uri": map[string]interface{}{
									"prefix": sharePath,
								},
							},
						},
						"route": []interface{}{
							map[string]interface{}{
								"destination": map[string]interface{}{
									"host": fmt.Sprintf("%s%s.%s.svc.cluster.local",
										resourcePrefix, viewer.Name, viewer.Namespace),
									"port": map[string]interface{}{
										"number": int64(servicePort),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(viewer, vs, r.Scheme); err != nil {
		return err
	}

	return r.createOrUpdateUnstructured(ctx, log, vs, "Share VirtualService")
}

// reconcileShareEnvoyFilter creates or updates the EnvoyFilter to bypass authentication
func (r *PVCViewerReconciler) reconcileShareEnvoyFilter(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
) error {
	efName := fmt.Sprintf("bypass-auth-pvcviewer-share-%s-%s",
		viewer.Namespace, viewer.Name)
	routeName := fmt.Sprintf("pvcviewer-%s-%s-share-public",
		viewer.Namespace, viewer.Name)

	ef := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1alpha3",
			"kind":       "EnvoyFilter",
			"metadata": map[string]interface{}{
				"name":      efName,
				"namespace": "istio-system",
				"labels": map[string]string{
					pvcviewerNameLabelKey:      viewer.Name,
					pvcviewerNamespaceLabelKey: viewer.Namespace,
				},
			},
			"spec": map[string]interface{}{
				"workloadSelector": map[string]interface{}{
					"labels": map[string]interface{}{
						"istio": "ingressgateway",
					},
				},
				"configPatches": []interface{}{
					// HTTP port 8080
					createEnvoyFilterPatch(routeName, "*:8080"),
					// HTTPS port 443
					createEnvoyFilterPatch(routeName, "*:443"),
				},
			},
		},
	}

	// Note: EnvoyFilter in istio-system cannot use cross-namespace OwnerReference
	return r.createOrUpdateUnstructured(ctx, log, ef, "Share EnvoyFilter")
}

// createEnvoyFilterPatch creates a config patch for EnvoyFilter
func createEnvoyFilterPatch(routeName, vhostName string) map[string]interface{} {
	return map[string]interface{}{
		"applyTo": "HTTP_ROUTE",
		"match": map[string]interface{}{
			"context": "GATEWAY",
			"routeConfiguration": map[string]interface{}{
				"vhost": map[string]interface{}{
					"name": vhostName,
					"route": map[string]interface{}{
						"name": routeName,
					},
				},
			},
		},
		"patch": map[string]interface{}{
			"operation": "MERGE",
			"value": map[string]interface{}{
				"typed_per_filter_config": map[string]interface{}{
					"envoy.filters.http.ext_authz": map[string]interface{}{
						"@type":    "type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthzPerRoute",
						"disabled": true,
					},
				},
			},
		},
	}
}

// reconcileShareAuthPolicyGateway creates or updates the Gateway AuthorizationPolicy
func (r *PVCViewerReconciler) reconcileShareAuthPolicyGateway(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
) error {
	apName := fmt.Sprintf("allow-pvcviewer-share-%s-%s-gw",
		viewer.Namespace, viewer.Name)
	sharePath := fmt.Sprintf("%s/%s/%s/share/*",
		viewer.Spec.Networking.BasePrefix,
		viewer.Namespace,
		viewer.Name)

	ap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "AuthorizationPolicy",
			"metadata": map[string]interface{}{
				"name":      apName,
				"namespace": "istio-system",
				"labels": map[string]string{
					pvcviewerNameLabelKey:      viewer.Name,
					pvcviewerNamespaceLabelKey: viewer.Namespace,
				},
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						"istio": "ingressgateway",
					},
				},
				"action": "ALLOW",
				"rules": []interface{}{
					map[string]interface{}{
						"to": []interface{}{
							map[string]interface{}{
								"operation": map[string]interface{}{
									"paths": []string{sharePath},
								},
							},
						},
					},
				},
			},
		},
	}

	return r.createOrUpdateUnstructured(ctx, log, ap, "Share AuthorizationPolicy (Gateway)")
}

// reconcileShareAuthPolicyBackend creates or updates the Backend AuthorizationPolicy
func (r *PVCViewerReconciler) reconcileShareAuthPolicyBackend(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
	commonLabels map[string]string,
) error {
	apName := fmt.Sprintf("allow-pvcviewer-share-%s-inbound", viewer.Name)

	// Fixed allowed HTTP methods (read-only operations)
	methods := []string{"GET", "HEAD", "OPTIONS"}

	// Path configuration: need to match both external and internal paths
	externalPath := fmt.Sprintf("%s/%s/%s/share/*",
		viewer.Spec.Networking.BasePrefix,
		viewer.Namespace,
		viewer.Name)
	internalPath := "/share/*"

	ap := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "security.istio.io/v1",
			"kind":       "AuthorizationPolicy",
			"metadata": map[string]interface{}{
				"name":      apName,
				"namespace": viewer.Namespace,
				"labels":    commonLabels,
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{
					"matchLabels": map[string]interface{}{
						instanceLabelKey: resourcePrefix + viewer.Name,
					},
				},
				"action": "ALLOW",
				"rules": []interface{}{
					map[string]interface{}{
						"to": []interface{}{
							map[string]interface{}{
								"operation": map[string]interface{}{
									"methods": methods,
									"paths":   []string{externalPath, internalPath},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(viewer, ap, r.Scheme); err != nil {
		return err
	}

	return r.createOrUpdateUnstructured(ctx, log, ap, "Share AuthorizationPolicy (Backend)")
}

// createOrUpdateUnstructured creates or updates an unstructured resource
func (r *PVCViewerReconciler) createOrUpdateUnstructured(
	ctx context.Context,
	log logr.Logger,
	obj *unstructured.Unstructured,
	resourceType string,
) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(obj.GroupVersionKind())

	err := r.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, existing)

	if err != nil {
		if apierrs.IsNotFound(err) {
			log.Info(fmt.Sprintf("Creating %s", resourceType),
				"name", obj.GetName(),
				"namespace", obj.GetNamespace())
			return r.Create(ctx, obj)
		}
		return err
	}

	// Update resource
	obj.SetResourceVersion(existing.GetResourceVersion())
	log.Info(fmt.Sprintf("Updating %s", resourceType),
		"name", obj.GetName(),
		"namespace", obj.GetNamespace())
	return r.Update(ctx, obj)
}

// cleanupIstioSystemResources removes public share resources from istio-system namespace
func (r *PVCViewerReconciler) cleanupIstioSystemResources(
	ctx context.Context,
	log logr.Logger,
	viewer *kubefloworgv1alpha1.PVCViewer,
) error {
	// Delete EnvoyFilter
	efName := fmt.Sprintf("bypass-auth-pvcviewer-share-%s-%s",
		viewer.Namespace, viewer.Name)
	ef := &unstructured.Unstructured{}
	ef.SetAPIVersion("networking.istio.io/v1alpha3")
	ef.SetKind("EnvoyFilter")
	ef.SetName(efName)
	ef.SetNamespace("istio-system")
	if err := r.Delete(ctx, ef); err != nil && !apierrs.IsNotFound(err) {
		log.Error(err, "Failed to delete EnvoyFilter", "name", efName)
		return err
	}

	// Delete Gateway AuthorizationPolicy
	apName := fmt.Sprintf("allow-pvcviewer-share-%s-%s-gw",
		viewer.Namespace, viewer.Name)
	ap := &unstructured.Unstructured{}
	ap.SetAPIVersion("security.istio.io/v1")
	ap.SetKind("AuthorizationPolicy")
	ap.SetName(apName)
	ap.SetNamespace("istio-system")
	if err := r.Delete(ctx, ap); err != nil && !apierrs.IsNotFound(err) {
		log.Error(err, "Failed to delete AuthorizationPolicy (Gateway)", "name", apName)
		return err
	}

	log.Info("Successfully cleaned up istio-system resources")
	return nil
}

// containsString checks if a string is present in a slice
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a slice
func removeString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
