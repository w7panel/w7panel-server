package webhook

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 Deployment 资源

// 处理 Ingress 资源
func (m *ResourceMutator) handleNode(ctx context.Context, req admission.Request) admission.Response {
	slog.Info("处理 node admission 请求")
	// 解码请求中的 Ingress 资源

	// if req.Operation == "DELETE" {
	// 	return admission.Allowed("")
	// }

	node := &v1.Node{}
	if req.Operation == "DELETE" {

		if err := (m.decoder).DecodeRaw(req.OldObject, node); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
		longhorn.WebHookDeleteNode(node)
	}
	return admission.Allowed("")

}
