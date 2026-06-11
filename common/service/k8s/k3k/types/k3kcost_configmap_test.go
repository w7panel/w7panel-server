package types

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCostMap(t *testing.T) {
	sdk := k8s.NewK8sClient()
	configmap, err := sdk.ClientSet.CoreV1().ConfigMaps("default").Get(sdk.Ctx, "k3k.zjvjdnnp", v1.GetOptions{})
	if err != nil {
		t.Error(err)
	}
	t.Log(configmap)
	kconfig := NewK3kCostConfigMap(configmap)
	kconfig.getCost()
}
