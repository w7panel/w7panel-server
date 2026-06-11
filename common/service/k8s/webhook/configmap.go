package webhook

import (
	"context"
	"net/http"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 Deployment 资源

// 处理 Ingress 资源
func (m *ResourceMutator) handleConfigmap(ctx context.Context, req admission.Request) admission.Response {
	// slog.Info("处理 Ingress admission 请求")
	// 解码请求中的 Ingress 资源

	// if req.Operation == "DELETE" {
	// 	return admission.Allowed("")
	// }

	configmap := &v1.ConfigMap{}
	if err := (m.decoder).Decode(req, configmap); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	defer checkLogo(configmap)
	defer checkPublish(m.client, configmap.DeepCopy())
	return admission.Allowed("")
}

func checkLogo(configMap *v1.ConfigMap) {
	time.AfterFunc(1*time.Second, func() {
		k8s.WriteLogo(configMap)
	})
}

// 发布到s.w7.cc
func checkPublish(client client.Client, configMap *v1.ConfigMap) {
	time.AfterFunc(1*time.Second, func() {
		if configMap.Labels != nil && configMap.Labels["type"] == "cost" {
			k3k.CheckPublish(context.Background(), client, configMap)
		}
	})

}
