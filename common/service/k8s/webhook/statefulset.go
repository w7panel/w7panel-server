package webhook

import (
	"context"
	"log/slog"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 StatefulSet 资源
func (m *ResourceMutator) handleStatefulSet(ctx context.Context, req admission.Request) admission.Response {
	slog.Info("处理 StatefulSet admission 请求")

	// 解码请求中的 StatefulSet 资源
	statefulset := &appsv1.StatefulSet{}
	if err := (m.decoder).Decode(req, statefulset); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	ResetImage(statefulset.Namespace, statefulset.Name, "StatefulSet", statefulset.Annotations)

	return admission.Allowed("无需修改 statefulset")
}
