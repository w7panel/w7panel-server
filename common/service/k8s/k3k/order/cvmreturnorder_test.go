package order

import (
	"context"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
)

func TestReturnOrder(t *testing.T) {

	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		panic(err)
	}
	cvm, err := k3k.GetCvm(context.Background(), sdk, "k3k-console-56416", "console-56416-cjafo")
	if err != nil {
		panic(err)
	}
	orderApi.LockReturnLastOrder(context.Background(), cvm, true)
}
