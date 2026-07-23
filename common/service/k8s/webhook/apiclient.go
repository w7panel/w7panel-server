package webhook

import (
	"context"
	"encoding/json"

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
			if item.Spec.TokenType == "" {
				item.Spec.TokenType = apiclientv1alpha1.TokenTypeTemporary
			}
			if !k8sapiclient.IsValidTokenType(item.Spec.TokenType) {
				return admission.Denied("tokenType must be temporary or permanent")
			}
			if item.Spec.TokenType == apiclientv1alpha1.TokenTypeTemporary && item.Spec.TemporaryTokenMinutes == nil {
				defaultMinutes := k8sapiclient.DefaultTemporaryTokenMinutes
				item.Spec.TemporaryTokenMinutes = &defaultMinutes
			}
			if item.Spec.TemporaryTokenMinutes != nil {
				if *item.Spec.TemporaryTokenMinutes < k8sapiclient.MinTemporaryTokenMinutes || *item.Spec.TemporaryTokenMinutes > k8sapiclient.MaxTemporaryTokenMinutes {
					return admission.Denied("temporaryTokenMinutes must be between 1 and 1440")
				}
			}
			if req.Operation == "UPDATE" {
				oldItem := &apiclientv1alpha1.ApiClient{}
				if err := m.decoder.DecodeRaw(req.OldObject, oldItem); err == nil {
					if oldItem.Spec.ClientID != "" && oldItem.Spec.ClientID != item.Spec.ClientID {
						return admission.Denied("clientId is immutable")
					}
					if k8sapiclient.NormalizeTokenType(oldItem.Spec.TokenType) != k8sapiclient.NormalizeTokenType(item.Spec.TokenType) {
						return admission.Denied("tokenType is immutable")
					}
				}
			}
			k8sapiclient.UpsertCache(item)
			if item.Spec.TokenType != "" {
				marshaled, err := json.Marshal(item)
				if err != nil {
					return admission.Errored(500, err)
				}
				return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)
			}
		}
	}

	return admission.Allowed("api client cache synced")
}
