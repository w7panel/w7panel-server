package console

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/order"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	cvmList := &cvmv1alpha1.CvmList{}
	err = sigClient.List(context.TODO(), cvmList)
	if err != nil {
		slog.Error("return check list find err", "err", err)
		os.Exit(1)
	}
	c.handleCvm(cvmList, sdk.Sdk, sigClient)
}

func (K3kOrderReturnCheck) handleCvm(cvmList *cvmv1alpha1.CvmList, sdk *k8s.Sdk, client sigclient.Client) error {
	orderApi, err := order.NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	for _, cvm := range cvmList.Items {
		err := loadReturnCvmOrder(&cvm, orderApi, client)
		if err != nil {
			slog.Error("load return cvm order err", "err", err)
			continue
		}
	}
	return nil
}

func loadReturnCvmOrder(cvm *cvmv1alpha1.Cvm, orderApi *order.K3kOrderApi, client sigclient.Client) error {

	returnOrder, err := orderApi.FindLastReturnCvmOrder(cvm)
	if err != nil {
		slog.Error("last return order find err", "err", err)
		return err
	}
	if !returnOrder.HasOrder {
		slog.Error("cvm has no return order", "name", cvm.Name)
		return nil
	}
	consoleOrder := &cvmv1alpha1.CvmConsoleOrder{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CvmConsoleOrder",
			APIVersion: "cvm.w7.cc/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: strings.ToLower(returnOrder.K3kOrder.OrderSn),
		},
	}
	_, err = controllerutil.CreateOrPatch(context.Background(), client, consoleOrder, func() error {
		consoleOrder.Spec = cvmv1alpha1.CvmConsoleOrderSpec{
			CvmName: cvm.Name,
			Order: &cvmv1alpha1.CvmOrder{
				OrderSn: returnOrder.K3kOrder.OrderSn,
				Status:  returnOrder.K3kOrder.OrderStatus,
				Resource: &cvmv1alpha1.CvmResource{
					CPU:       returnOrder.K3kOrder.Cpu,
					Memory:    returnOrder.K3kOrder.Memory,
					Storage:   returnOrder.K3kOrder.Storage,
					Bandwidth: returnOrder.K3kOrder.Bandwidth,
				},
				BuyMode:      returnOrder.K3kOrder.BuyMode,
				Hour:         int(returnOrder.K3kOrder.GetHour()),
				ReturnFinish: false,
			},
		}
		return nil
	})
	if err != nil {
		slog.Error("create or patch console order err", "err", err)
		return err
	}
	return nil

}
