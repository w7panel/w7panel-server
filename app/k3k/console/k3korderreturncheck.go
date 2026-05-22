package console

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	cvmv1alpha1 "github.com/w7panel/w7panel-ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/order"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type K3kOrderReturnCheck struct {
	console2.Abstract
}

func (c K3kOrderReturnCheck) GetName() string {
	return "k3k-return-check"
}

func (c K3kOrderReturnCheck) Configure(cmd *cobra.Command) {

}

func (c K3kOrderReturnCheck) GetDescription() string {
	return "退款记录"
}

func (c K3kOrderReturnCheck) Handle(cmd *cobra.Command, args []string) {

	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("Failed to create sigclient", "error", err)
		return
	}
	cvmList := &cvmv1alpha1.CkmList{}
	err = sigClient.List(context.TODO(), cvmList)
	if err != nil {
		slog.Error("return check list find err", "err", err)
		os.Exit(1)
	}
	c.handleCvm(cvmList, sdk.Sdk, sigClient)
}

func (K3kOrderReturnCheck) handleCvm(cvmList *cvmv1alpha1.CkmList, sdk *k8s.Sdk, client sigclient.Client) error {
	orderApi, err := order.NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	for _, cvm := range cvmList.Items {
		err := fixReturnCvmOrder(&cvm, orderApi, client)
		if err != nil {
			slog.Error("load return cvm order err", "err", err)
			continue
		}
	}
	return nil
}
func fixReturnCvmOrder(cvm *cvmv1alpha1.Ckm, orderApi *order.K3kOrderApi, client sigclient.Client) error {

	return nil
	// err := orderApi.LockReturnLastOrder(context.Background(), cvm, true)
	// if err != nil {
	// 	return err
	// }
	// return nil
}
