package order

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (k *K3kOrderApi) getCvmConsoleOrder(user *types.K3kUser, orderSn string) (*cvmv1alpha1.CvmConsoleOrder, error) {
	orderSn = strings.ToLower(orderSn)
	cvmOrder := &cvmv1alpha1.CvmConsoleOrder{}
	if err := k.client.Get(k.sdk.Ctx, client.ObjectKey{Name: orderSn, Namespace: user.GetK3kNamespace()}, cvmOrder); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, err
	}
	return cvmOrder, nil
}
func (k *K3kOrderApi) CreateBaseResourceCvmOrder(baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {
	// cvm, err := k.getCvm(user, baseResource.CvmName)
	// if err != nil {
	// 	return nil, err
	// }
	// cvmOrder, err := newCvmOrder(cvm, user)
	// if err != nil {
	// 	return nil, err
	// }
	// if cvmOrder.CanBaseBuy() == false {
	// 	return nil, errors.New("不支持购买")
	// }

	// if err := cvmOrder.CanCreateBaseOrderError(); err != nil {
	// 	return nil, err
	// }
	cvmName := baseResource.CvmName

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
	params["cvm_name"] = cvmName
	if err := k.LockCoupon(conponCode, used); err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, BASE_BUY)
	if err != nil {
		return nil, err
	}
	consoleOrder := &cvmv1alpha1.CvmConsoleOrder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strings.ToLower(result.OrderSn),
			Namespace: user.GetK3kNamespace(),
			Labels: map[string]string{
				"w7.cc/cvm-name": cvmName,
			},
		},
		Spec: cvmv1alpha1.CvmConsoleOrderSpec{
			Order: &cvmv1alpha1.CvmOrder{
				OrderSn: result.OrderSn,
			},
			CvmName: cvmName,
		},
	}
	err = k.client.Create(k.sdk.Ctx, consoleOrder)
	if err != nil {
		return nil, err
	}
	if err := k.UsedCoupon(conponCode, used, result.OrderSn); err != nil {
		slog.Error("used coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("used coupon code error")
	}
	if !result.NeedPay {
		time.AfterFunc(time.Second*2, func() {
			_ = k.NotifyCvmOrder(user, cvmName, result.OrderSn)
		})
	}
	return result, nil
}
