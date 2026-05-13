package order

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	cvmv1alpha1 "cnb.cool/i0358/ai-cvm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func currentCvmBuyResource(cvm *cvmv1alpha1.Cvm) types.BuyResource {
	resource := cvm.Status.EffectiveResource
	if resource == nil {
		resource = &cvmv1alpha1.CvmResource{}
	}
	return types.BuyResource{
		Cpu:       resource.CPU,
		Memory:    resource.Memory,
		Storage:   resource.Storage,
		Bandwidth: resource.Bandwidth,
	}
}
func newCvmOrder(cvm *cvmv1alpha1.Cvm, user *types.K3kUser) (*types.K3kCvmOrder, error) {
	cost := user.GetCost()
	if cost == nil {
		return nil, fmt.Errorf("当前用户未配置费用套餐，无法购买")
	}
	return types.Newk3kCvmOrder(cvm, types.Newk3kCvmOverSelling(cvm), types.Newk3kCvmTime(cvm), cost), nil
}

func getOrderSnByName(orderName string, w7config *config.W7Config, orderSn string) (*console.OrderInfo, error) {
	values := url.Values{}
	values.Set("orderSn", orderSn)
	values.Set("k3kName", orderName)
	apiClient := console.NewConsoleCdClient(w7config.ThirdpartyCDToken)
	info, err := apiClient.GetPanelOrderInfo(values)
	if err != nil {
		return nil, err
	}
	return info, nil
}

func MockNotifyOrderCvm(user *types.K3kUser, sn string) error {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	return orderApi.MockNotifyPaidOrderCvm(user, sn)
}

func createCrdOrder(ctx context.Context, client client.Client, orderSn, namespace, cvmName string, isDemo bool) (*cvmv1alpha1.CvmConsoleOrder, error) {
	consoleOrder := &cvmv1alpha1.CvmConsoleOrder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(orderSn),
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/cvm-name": cvmName,
			},
		},
		Spec: cvmv1alpha1.CvmConsoleOrderSpec{
			Order: &cvmv1alpha1.CvmOrder{
				OrderSn: orderSn,
			},
			CvmName: cvmName,
		},
	}
	if isDemo {
		consoleOrder.Labels["w7.cc/demo"] = "true" //演示资源
	}
	err := client.Create(ctx, consoleOrder)
	if err != nil {
		return nil, err
	}
	return consoleOrder, nil
}
