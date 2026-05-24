package controllers

import (
	"context"
	"net/http"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	notebookv1 "github.com/kubeflow/kubeflow/components/notebook-controller/api/v1beta1"

	"slices"
)

// +kubebuilder:webhook:path=/validate-kubeflow-org-v1beta1-notebook,mutating=false,failurePolicy=fail,sideEffects=None,groups=kubeflow.org,resources=notebooks,verbs=create;update,versions=v1beta1;v1,name=vnotebook.kb.io,admissionReviewVersions=v1

var TargetAnnotationKeys = []string{
	"kflow.cgu.com.tw/external-access",
	"kflow.cgu.com.tw/proxy-allow-ports",
	"kflow.cgu.com.tw/proxy-ports",
}

// NotebookValidator validates Notebook based on annotations
type NotebookValidator struct {
	Client     client.Client
	decoder    *admission.Decoder
	AdminUsers []string
}

// targetAnnotationChanged checks if any of the target annotations have been added or modified.
func targetAnnotationChanged(oldObj, newObj *notebookv1.Notebook) (bool, []string) {
	var changedKeys []string

	for _, key := range TargetAnnotationKeys {

		var oldVal, newVal string
		var oldExists, newExists bool

		if oldObj != nil && oldObj.Annotations != nil {
			oldVal, oldExists = oldObj.Annotations[key]
		}

		if newObj != nil && newObj.Annotations != nil {
			newVal, newExists = newObj.Annotations[key]
		}

		if oldExists != newExists || oldVal != newVal {
			changedKeys = append(changedKeys, key)
		}
	}

	return len(changedKeys) > 0, changedKeys
}

// isAdmin checks if the user is in the configured AdminUsers list.
func (v *NotebookValidator) isAdmin(username string) bool {
	for _, admin := range v.AdminUsers {
		if username == admin {
			return true
		}
	}
	return false
}

// Handle handles admission requests.
func (v *NotebookValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("webhook").WithValues("username", req.UserInfo.Username, "groups", req.UserInfo.Groups)
	logger.Info("Received admission request for notebook", "operation", req.Operation)

	notebook := &notebookv1.Notebook{}

	// Parse the incoming object
	err := v.decoder.Decode(req, notebook)
	if err != nil {
		logger.Error(err, "Failed to decode incoming object")
		return admission.Errored(http.StatusBadRequest, err)
	}

	var oldNotebook *notebookv1.Notebook
	if req.Operation == "UPDATE" {
		oldNotebook = &notebookv1.Notebook{}
		err = v.decoder.DecodeRaw(req.OldObject, oldNotebook)
		if err != nil {
			logger.Error(err, "Failed to decode old object")
			return admission.Errored(http.StatusBadRequest, err)
		}
	} else if req.Operation == "CREATE" {
		oldNotebook = nil
	} else {
		return admission.Allowed("") // Only handle CREATE/UPDATE
	}

	// Check if the target annotations have been added or modified
	changed, changedKeys := targetAnnotationChanged(oldNotebook, notebook)
	if changed {
		logger.Info("Target annotation has changed", "annotations", changedKeys)

		// If changed, check if the user is an admin
		if !v.isAdmin(req.UserInfo.Username) {
			logger.Info("Access denied: user is not in admin list", "adminUsersConfigured", v.AdminUsers)
			return admission.Denied("Permission Denied: Only admins can add or modify annotations: " + strings.Join(changedKeys, ", "))
		}

		if slices.Contains(changedKeys, "kflow.cgu.com.tw/proxy-ports") {
			var newPorts = strings.ReplaceAll(notebook.Annotations["kflow.cgu.com.tw/proxy-ports"], " ", "")
			for _, port := range strings.Split(newPorts, ",") {
				var allowPorts = strings.ReplaceAll(notebook.Annotations["kflow.cgu.com.tw/proxy-allow-ports"], " ", "")
				if !slices.Contains(strings.Split(allowPorts, ","), port) {
					return admission.Denied("Annotations should belongs to proxy-allow-ports.")
				}
			}
		}

		logger.Info("Access granted")
	}

	return admission.Allowed("")
}

// InjectDecoder injects the decoder.
func (v *NotebookValidator) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
