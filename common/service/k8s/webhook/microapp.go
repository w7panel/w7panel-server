package webhook

import (
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	microapp "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// not use in webhook
func (m *ResourceMutator) handleMicroApp(ctx context.Context, req admission.Request) admission.Response {
	// slog.Info("处理 MicroApp admission 请求")
	if req.Kind.Group == "microapp.w7.cc" {
		return m.handleLegacyMicroApp(ctx, req)
	}

	modified := false
	microApp := &microapp.MicroApp{}
	if err := (m.decoder).Decode(req, microApp); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	modified = SetControllerReference(microApp, m.client)

	if !modified {
		return admission.Allowed("")
	}
	md, err := json.Marshal(microApp)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, md)
}

func (m *ResourceMutator) handleLegacyMicroApp(ctx context.Context, req admission.Request) admission.Response {
	if req.DryRun != nil && *req.DryRun {
		return admission.Allowed("legacy microapp dry-run skipped")
	}

	item := &unstructured.Unstructured{}
	if err := item.UnmarshalJSON(req.Object.Raw); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	item.SetAPIVersion("w7panel.w7.com/v1alpha1")
	item.SetKind("MicroApp")
	if item.GetNamespace() == "" {
		item.SetNamespace(req.Namespace)
	}
	item.SetUID("")
	item.SetResourceVersion("")
	item.SetGeneration(0)
	item.SetCreationTimestamp(metav1.Time{})
	item.SetManagedFields(nil)
	item.SetOwnerReferences(nil)

	if group, err := appgroup.GetAppgroup(item.GetName(), item.GetNamespace(), m.client); err == nil {
		if err := controllerutil.SetControllerReference(group, item, k8s.GetScheme()); err != nil {
			slog.Error("set mirrored microapp controller reference failed", "namespace", item.GetNamespace(), "name", item.GetName(), "error", err)
		}
	}

	annotations := item.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		item.SetAnnotations(annotations)
	}

	if err := m.client.Patch(ctx, item, sigclient.Apply, sigclient.FieldOwner("w7panel-webhook"), sigclient.ForceOwnership); err != nil {
		slog.Error("mirror legacy microapp to w7panel group failed", "namespace", item.GetNamespace(), "name", item.GetName(), "error", err)
		return admission.Allowed("legacy microapp mirror failed")
	}

	return admission.Allowed("legacy microapp mirrored to w7panel group")
}

func SetControllerReference(microApp *microapp.MicroApp, client sigclient.Client) bool {
	if !controllerutil.HasControllerReference(microApp) {
		// return admission.Allowed("")
		group, err := appgroup.GetAppgroup(microApp.Name, microApp.Namespace, client)
		if err != nil {
			return false
		}
		err = controllerutil.SetControllerReference(group, microApp, k8s.GetScheme())
		if err != nil {
			slog.Error("SetControllerReference error", "error", err)
			return false
		}
		return true
	}
	return false
}
