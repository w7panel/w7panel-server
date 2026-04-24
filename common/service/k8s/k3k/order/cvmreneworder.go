package order

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (k *K3kOrderApi) CreateRenewCvmOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {
	cvm, err := k.getCvm(user, baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return nil, err
	}
	// if err := cvmOrder.CanRenewError(); err != nil {
	// 	return nil, err
	// }
	if cvmOrder.CanRenew() == false {
		return nil, errors.New("不支持续费")
	}
	currentResource := currentCvmBuyResource(cvm)
	compute := types.NewK3kOrderCompute(currentResource, baseResource.UnitQuantity, user.GetCost(), nil)
	conponCode := baseResource.CouponCode
	compute = k.applyCoupon(compute, conponCode, user)
	used := compute.IsCouponMatch()
	params := compute.ToReqParams()
	params["buymode"] = RENEW_BUY
	params["price"] = compute.GetDiscountPrice(RENEW_BUY).String()
	params["hour"] = strconv.FormatFloat(baseResource.GetHours(), 'f', 2, 64)
	params["cvm_name"] = cvm.Name
	if err := k.LockCoupon(conponCode, used); err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, RENEW_BUY)
	if err != nil {
		return nil, err
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
		cvmOrder.SetRenewOrder(result.OrderSn)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := k.UsedCoupon(conponCode, used, result.OrderSn); err != nil {
		slog.Error("used coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("used coupon code error")
	}
	if !result.NeedPay {
		time.AfterFunc(time.Second*2, func() {
			_ = k.NotifyCvmOrder(user, cvm.Name, result.OrderSn)
		})
	}
	return result, nil
}
