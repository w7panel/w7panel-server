package console

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	cvmv1alpha1 "github.com/w7panel/w7panel/common/service/k8s/ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/order"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"k8s.io/apimachinery/pkg/types"
)

type K3kOrderReturnCheckOne struct {
	console2.Abstract
}

type shellOption struct {
	cvmName   string
	namespace string
}

var shOp = shellOption{}

// go run main.go k3k-return-check-one --sa=console-303483
func (c K3kOrderReturnCheckOne) GetName() string {
	return "k3k-return-check-one"
}

func (c K3kOrderReturnCheckOne) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&shOp.cvmName, "cvmName", "", "cvmName")
	cmd.Flags().StringVar(&shOp.namespace, "namespace", "", "namespace")
}

func (c K3kOrderReturnCheckOne) GetDescription() string {
	return "退款记录除了里"
}

func (c K3kOrderReturnCheckOne) Handle(cmd *cobra.Command, args []string) {

	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("Failed to create sigclient", "error", err)
		return
	}
	cvm := &cvmv1alpha1.Ckm{}
	err = sigClient.Get(context.TODO(), types.NamespacedName{Name: shOp.cvmName, Namespace: shOp.namespace}, cvm)
	if err != nil {
		slog.Error("return check list find err", "err", err)
		os.Exit(1)
	}
	orderApi, err := order.NewK3kOrderApi(sdk.Sdk)
	if err != nil {
		slog.Error("order api init err", "err", err)
		os.Exit(1)
	}
	err = fixReturnCvmOrder(cvm, orderApi, sigClient)
	if err != nil {
		slog.Error("fix return cvm order err", "err", err)
		os.Exit(1)
	}

}
