package webhook

import (
	"context"
	"net/http"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 Service 资源
func (m *ResourceMutator) handleServiceAccount(ctx context.Context, req admission.Request) admission.Response {
	// slog.Info("处理 ServiceAccount admission 请求")

	if req.Operation != "UPDATE" {
		return admission.Allowed("无需修改 ServiceAccount")
	}

	sa := &v1.ServiceAccount{}
	if err := (m.decoder).Decode(req, sa); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	return admission.Allowed("无需修改 ServiceAccount")
}
