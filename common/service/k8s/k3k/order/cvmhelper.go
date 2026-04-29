package order

import (
	"fmt"
	"net/url"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
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
