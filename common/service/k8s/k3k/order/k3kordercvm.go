package order

import (
	"context"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateBaseResourceCvmOrder(ctx context.Context, baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	// 如果没传cvm name 会自动创建一个

	if baseResource.CvmName == "" {
		baseResource.CvmName = helper.RandomString(10)
		_, err := orderApi.getCvm(user, baseResource.CvmName)
		if err != nil {
			if !errors.IsNotFound(err) {
				return nil, err
			}
			cvm := &cvmv1alpha1.Cvm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      baseResource.CvmName,
					Namespace: user.GetK3kNamespace(),
				},
				Spec: cvmv1alpha1.CvmSpec{
					ProvisionMode: "order-required",
				},
			}
			sigClient, err := sdk.ToSigClient()
			if err != nil {
				return nil, err
			}
			err = sigClient.Create(ctx, cvm)
			if err != nil {
				return nil, err
			}
		}
	}

	err = orderApi.CheckCanBuy(user)
	if err != nil {
		return nil, err
	}
	return orderApi.CreateBaseResourceCvmOrder(baseResource, user)
}

func CreateRenewCvmOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	err = orderApi.CheckCanBuyCvm(baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	return orderApi.CreateRenewCvmOrder(baseResource, user)

}

func CreateExpandCvmOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return nil, err
	}
	err = orderApi.CheckCanBuyCvm(baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	return orderApi.CreateExpandCvmOrder(baseResource, user)

}
