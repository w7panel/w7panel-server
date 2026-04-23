package order

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	v1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

func TestCreateRenewResourceOrderCvm(t *testing.T) {

	// os.Setenv("LOCAL_MOCK", "true")
	os.Setenv("USER_AGENT", "we7test-beta")
	// os.Setenv("DEBUG", "true")

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
	needBuy := k3kUser.NeedBuyResource()
	t.Log(needBuy)
	pay, err := CreateRenewCvmOrder(&types.BuyRenewResource{BaseConfigName: "admin", CvmName: "jshinqhsvd", UnitQuantity: types.UnitQuantity{Unit: "month", Quantity: 3}}, k3kUser)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(pay)

	// NotifyPaid(k3kUser)
	// t.Log(pay)
}
