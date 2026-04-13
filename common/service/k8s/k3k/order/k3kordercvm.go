package order

import (
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
)

func CreateBaseResourceCvmOrder(baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	err = orderApi.CheckCanBuy(user)
	if err != nil {
		return nil, err
	}
	err = user.CanCreateBaseOrderError()
	if err != nil {
		return nil, err
	}
	return orderApi.CreateBaseResourceOrder(baseResource, user)
}

func CreateRenewCvmOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	err = orderApi.CheckCanBuy(user)
	if err != nil {
		return nil, err
	}
	err = user.CanRenewError()
	if err != nil {
		return nil, err
	}
	return orderApi.CreateRenewOrder(baseResource, user)

}

func CreateExpandCvmOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	err = orderApi.CheckCanBuy(user)
	if err != nil {
		return nil, err
	}
	err = user.CanExpandError()
	if err != nil {
		return nil, err
	}
	return orderApi.CreateExpandOrder(baseResource, user)

}
