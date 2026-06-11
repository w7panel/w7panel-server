package webhook

import (
	"context"

	k8sapiclient "github.com/w7panel/w7panel/common/service/k8s/apiclient"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (m *ResourceMutator) handleApiClient(ctx context.Context, req admission.Request) admission.Response {
	switch req.Operation {
	case "DELETE":
		item := &apiclientv1alpha1.ApiClient{}
		if err := m.decoder.DecodeRaw(req.OldObject, item); err == nil {
			k8sapiclient.DeleteCache(item.Namespace, item.Name)
		}
	default:
		item := &apiclientv1alpha1.ApiClient{}
		if err := m.decoder.Decode(req, item); err == nil {
			k8sapiclient.UpsertCache(item)
		}
	}

	return admission.Allowed("api client cache synced")
}
