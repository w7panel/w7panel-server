package webhook

import (
	"context"
	"net/http"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 Daemonset 资源
func (m *ResourceMutator) handleDaemonset(ctx context.Context, req admission.Request) admission.Response {
	// slog.Info("处理 Daemonset admission 请求")

	// 解码请求中的 StatefulSet 资源
	ds := &appsv1.DaemonSet{}
	if err := (m.decoder).Decode(req, ds); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	ResetImage(ds.Namespace, ds.Name, "DaemonSet", ds.Annotations)

	return admission.Allowed("无需修改 Daemonset")
}
