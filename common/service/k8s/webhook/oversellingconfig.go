package webhook

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (m *ResourceMutator) handleOverSellingConfig(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == "DELETE" {
		return admission.Allowed("")
	}

	config := &configv1alpha1.OverSellingConfig{}
	if err := m.decoder.Decode(req, config); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	defer checkOverSellingConfig(config)
	return admission.Allowed("")
}

func checkOverSellingConfig(config *configv1alpha1.OverSellingConfig) {
	time.AfterFunc(1*time.Second, func() {
		longhorn.LonghorStoragePercentage(strconv.Itoa(int(config.Spec.Storage)))
	})
}
