package webhook

import (
	"context"
	"net/http"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 处理 secret 资源
func (m *ResourceMutator) handleSecret(ctx context.Context, req admission.Request) admission.Response {
	// slog.Error("处理 secret admission 请求")

	// if req.Operation == "DELETE" {
	// 	return admission.Allowed("")
	// }
	secret := &v1.Secret{}
	// 判断是否Delete 请求
	if req.Operation == "DELETE" {
		if err := (m.decoder).DecodeRaw(req.OldObject, secret); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	} else {
		if err := (m.decoder).Decode(req, secret); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}
	}

	if helper.IsK3kVirtual() {
		defer k3k.SyncHttpAfter(secret, "sync-secret") // 同步到主集群
	}

	// if !delete { //cert-manager 已经创建到子集群了，不需要再同步
	// 	if !helper.IsChildAgent() {
	// 		time.AfterFunc(time.Second*10, func() {
	// 			k3k.SyncToChildSecret(secret.DeepCopy()) // 同步到子集群
	// 		})
	// 	}
	// }
	return admission.Allowed("处理 secret 请求")
}
