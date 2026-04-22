package order

import (
	"context"
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	v1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

func TestCreateBaseResourceOrderCvm(t *testing.T) {

	// os.Setenv("LOCAL_MOCK", "true")
	os.Setenv("USER_AGENT", "we7test-beta")
	// os.Setenv("LOCAL_MOCK", "1")
	os.Setenv("DEBUG", "true")
	// console.SetConsoleApi("http://172.16.1.116:9004")
	// Setup mock

	sdk := k8s.NewK8sClient().Sdk
	client, err := sdk.ToSigClient()
	if err != nil {
		t.Error(err)
		return
	}

	sa := &v1.ServiceAccount{}
	err = client.Get(sdk.Ctx, ktypes.NamespacedName{Namespace: "default", Name: "console-164315"}, sa)
	if err != nil {
		t.Error(err)
		return
	}
	k3kUser := types.NewK3kUser(sa)
	k3k.RefreshK3kUser(k3kUser, sdk, true)
	bs := types.BuyResource{
		Cpu:       3,
		Memory:    4,
		Storage:   5,
		Bandwidth: 50,
	}
	pay, err := CreateBaseResourceCvmOrder(context.Background(), &types.BuyBaseResource{CvmName: "jshinqhsvd", CouponCode: "", BaseConfigName: "admin", UnitQuantity: types.UnitQuantity{Unit: "month", Quantity: 1}, BuyResource: bs}, k3kUser)
	if err != nil {
		t.Error(err)
		return
	}

	// Refresh(k3kUser)
	t.Log(pay)
}

func TestMockNotifyCvm(t *testing.T) {

	os.Setenv("LOCAL_MOCK", "true")
	os.Setenv("USER_AGENT", "we7test-beta")
	os.Setenv("DEBUG", "true")

	// console.SetConsoleApi("http://172.16.1.116:9004")
	// Setup mock
	sdk := k8s.NewK8sClient().Sdk
	client, err := sdk.ToSigClient()
	if err != nil {
		t.Error(err)
		return
	}

	sa := &v1.ServiceAccount{}
	err = client.Get(sdk.Ctx, ktypes.NamespacedName{Namespace: "default", Name: "console-164315"}, sa)
	if err != nil {
		t.Error(err)
		return
	}
	k3kUser := types.NewK3kUser(sa)
	MockNotifyOrderCvm(k3kUser, "20260422194414PE26DK")

	// t.Log(pay)
}
