package webhook

import (
	"context"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (m *ResourceMutator) handleDeployment(ctx context.Context, req admission.Request) admission.Response {
	// slog.Info("处理 Deployment admission 请求")

	// 解码请求中的 Deployment 资源
	deployment := &appsv1.Deployment{}
	if err := (m.decoder).Decode(req, deployment); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ResetImage(deployment.Namespace, deployment.Name, "Deployment", deployment.Annotations)

	return admission.Allowed("无需修改 deployment")
}
