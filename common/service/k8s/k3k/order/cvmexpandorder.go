package order

import (
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (k *K3kOrderApi) CreateExpandCvmOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {
	cvm, err := k.getCvm(user, baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return nil, err
	}
	// if err := cvmOrder.CanExpandError(); err != nil {
	// 	return nil, err
	// }
	if cvmOrder.CanExpand() == false {
		return nil, errors.New("不支持扩容")
	}
	if err := baseResource.Valid(); err != nil {
		return nil, err
	}
	currentResource := currentCvmBuyResource(cvm)
	if baseResource.BuyResource.Less(currentResource) {
		return nil, fmt.Errorf("扩容资源小于当前购买资源")
	}
	diff := baseResource.BuyResource.Sub(currentResource)
	if err := diff.Valid(); err != nil {
		return nil, err
	}
	compute := types.NewK3kOrderCompute(diff, types.UnitQuantity{}, user.GetCost(), nil)
	expireTime, err := cvmOrder.GetExpireTime()
	if err != nil {
		return nil, err
	}
	if expireTime.IsZero() {
		return nil, fmt.Errorf("暂不支持单个资源购买")
	}
	if expireTime.Before(time.Now()) {
		return nil, fmt.Errorf("账户已过期，无法扩容")
	}
	sub := expireTime.Sub(time.Now())
	price, err := compute.GetExpandPrice(expireTime)
	if err != nil {
		return nil, err
	}
	params := compute.ToReqParams()
	params["buymode"] = EXPAND_BUY
	params["price"] = price.String()
	params["hour"] = decimal.NewFromInt32(int32(sub.Hours())).String()
	params["cvm_name"] = cvm.Name
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, EXPAND_BUY)
	if err != nil {
		return nil, err
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
		cvmOrder.SetExpandOrder(result.OrderSn)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !result.NeedPay {
		time.AfterFunc(time.Second*2, func() {
			_ = k.NotifyCvmOrder(user, cvm.Name, result.OrderSn)
		})
	}
	result.CvmName = cvm.Name
	result.CvmNamespace = cvm.Namespace
	return result, nil
}
