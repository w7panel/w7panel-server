package order

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (k *K3kOrderApi) getCvm(user *types.K3kUser, cvmName string) (*cvmv1alpha1.Cvm, error) {
	if cvmName == "" {
		return nil, fmt.Errorf("cvm不能为空")
	}
	cvm := &cvmv1alpha1.Cvm{}
	if err := k.client.Get(k.sdk.Ctx, client.ObjectKey{Name: cvmName, Namespace: user.GetK3kNamespace()}, cvm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, err
	}
	return cvm, nil
}
func (k *K3kOrderApi) CreateBaseResourceCvmOrder(baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {
	cvm, err := k.getCvm(user, baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return nil, err
	}
	if err := cvmOrder.CanCreateBaseOrderError(); err != nil {
		return nil, err
	}
	currentUq := baseResource.UnitQuantity
	if currentUq.IsEmpty() {
		return nil, fmt.Errorf("购买时长不能为空")
	}
	if err := baseResource.BuyResource.Valid(); err != nil {
		return nil, err
	}
	if baseResource.BuyResource.IsEmpty() {
		return nil, fmt.Errorf("至少购买一个资源")
	}
	compute := types.NewK3kOrderCompute(baseResource.BuyResource, currentUq, user.GetCost(), nil)
	conponCode := baseResource.CouponCode
	used := false
	if !compute.IsGiveInBaseBuyMode() {
		compute = k.applyCoupon(compute, conponCode, user)
		used = compute.IsCouponMatch()
	}
	params := compute.ToReqParams()
	params["buymode"] = BASE_BUY
	params["price"] = compute.GetDiscountPrice(BASE_BUY).String()
	params["hour"] = strconv.FormatFloat(currentUq.GetHours(), 'f', 2, 64)
	params["cvm_name"] = cvm.Name
	if err := k.LockCoupon(conponCode, used); err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, BASE_BUY)
	if err != nil {
		return nil, err
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
		cvmOrder.SetBaseOrder(result.OrderSn)
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
