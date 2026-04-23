package order

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	v1 "k8s.io/api/core/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
)

func TestCreateExpandResourceOrderCvm(t *testing.T) {

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
	bs := types.BuyResource{
		Cpu:       6,
		Memory:    8,
		Storage:   10,
		Bandwidth: 100,
	}
	pay, err := CreateExpandCvmOrder(&types.BuyExpandResource{BaseConfigName: "admin", CvmName: "jshinqhsvd", BuyResource: bs}, k3kUser)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(pay)

	// t.Log(pay)
}
