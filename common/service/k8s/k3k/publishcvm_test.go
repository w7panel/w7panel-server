// nolint
package k3k

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestCheckPublish(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	sdk := k8s.NewK8sClient()
	client, err := sdk.ToSigClient()
	if err != nil {
		t.Error(err)
		return
	}

	cfg := &corev1.ConfigMap{}
	if err := client.Get(sdk.Ctx, types.NamespacedName{Namespace: "default", Name: "k3k.uupcpzsz"}, cfg); err != nil {
		t.Error(err)
		return
	}
	// if cfg.Annotations["w7.cc/cost-name"] != "" {
	// 	configmap := &corev1.ConfigMap{}
	// 	err := client.Get(sdk.Ctx, types.NamespacedName{Namespace: "default", Name: cfg.Annotations["w7.cc/cost-name"]}, configmap)
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	cost, err := k3ktypes.ConfigMapToCost(configmap)
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	json, err := cost.ToJsonString()
	// 	if err != nil {
	// 		t.Error(err)
	// 		return
	// 	}
	// 	cfg.Annotations["w7.cc/cost"] = json
	// }
	// group := k3ktypes.NewK3kClusterPolicy(cfg)
	// CheckPublish(sdk.Ctx, client, cfg)
	// packages := group.GetCost().Packages
	// for _, pkg := range packages {
	// 	t.Log(pkg)
	// }

	CheckPublish(sdk.Ctx, client, cfg)
}
