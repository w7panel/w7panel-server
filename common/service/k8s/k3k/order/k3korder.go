package order

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const BaseOrderUrl = "https://console.w7.cc/api/v1/order"
const BASE_BUY = "base"     // 基础购买
const RENEW_BUY = "renew"   // 续费购买
const EXPAND_BUY = "expand" // 扩容购买

type K3kOrderApi struct {
	sdk              *k8s.Sdk
	client           client.Client
	consoleSdkClient *console.SdkClient
	w7respo          config.W7ConfigRepositoryInterface
	mu               sync.Mutex
}

func NewK3kOrderApi(sdk *k8s.Sdk) (*K3kOrderApi, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	consoleSdkClient, err := console.NewSdkClient(license)
	if err != nil {
		return nil, err
	}
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	w7respo := config.NewW7ConfigRepository(sdk)
	return &K3kOrderApi{
		sdk:              sdk,
		client:           sigClient,
		w7respo:          w7respo,
		consoleSdkClient: consoleSdkClient,
	}, nil
}
func (k *K3kOrderApi) getCoupon(code string) (*types.Coupon, error) {

	coupon, err := k.consoleSdkClient.GetCoupon(code)
	if err != nil {
		return nil, err
	}
	return types.ApiCouponToCoupon(coupon), nil
}

func (k *K3kOrderApi) LockCoupon(code string, used bool) error {
	if !used {
		return nil
	}
	err := k.consoleSdkClient.UpdateCoupon(code, "lock", "")
	if err != nil {
		return err
	}
	return nil
}
func (k *K3kOrderApi) UsedCoupon(code string, used bool, sn string) error {
	if !used {
		return nil
	}
	err := k.consoleSdkClient.UpdateCoupon(code, "used", sn)
	if err != nil {
		return err
	}
	return nil
}

func (k *K3kOrderApi) applyCoupon(compute *types.K3kOrderCompute, conponCode string, user *types.K3kUser) *types.K3kOrderCompute {
	if conponCode != "" {
		coupon, err := k.getCoupon(conponCode)
		if err != nil {
			slog.Error("getCoupon error", "err", err)
		}
		if coupon != nil && coupon.CanUse && coupon.GroupName == user.GetClusterPolicy() {
			compute = compute.WithCoupon(coupon)
		}
	}
	return compute
}

func (k *K3kOrderApi) getCurrentConfig(user *types.K3kUser) (*config.W7Config, error) {
	return k.w7respo.Get(user.Name)
}

func (k *K3kOrderApi) getMainConfig() (*config.W7Config, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	return k.w7respo.Get(license.FounderSaName)
}

func (k *K3kOrderApi) getCvm(user *types.K3kUser, cvmName string) (*cvmv1alpha1.Cvm, error) {
	if cvmName == "" {
		return nil, fmt.Errorf("cvm不能为空")
	}
	cvm := &cvmv1alpha1.Cvm{}
	if err := k.client.Get(k.sdk.Ctx, client.ObjectKey{Name: cvmName}, cvm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("cvm不存在")
		}
		return nil, err
	}
	if cvm.Annotations != nil {
		if namespace := cvm.Annotations[types.K3K_NAMESPACE]; namespace != "" && namespace != user.GetK3kNamespace() {
			return nil, fmt.Errorf("cvm不属于当前用户")
		}
	}
	return cvm, nil
}

func currentCvmBuyResource(cvm *cvmv1alpha1.Cvm) types.BuyResource {
	resource := cvm.Spec.Resource
	if resource.CPU == 0 && resource.Memory == 0 && resource.Storage == 0 && resource.Bandwidth == 0 {
		resource = cvm.Spec.BaseResource
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

func (k *K3kOrderApi) applyCouponToCvm(compute *types.K3kOrderCompute, conponCode string, user *types.K3kUser) *types.K3kOrderCompute {
	return k.applyCoupon(compute, conponCode, user)
}

func (k *K3kOrderApi) createOrderByName(baseConfigName string, currentConfigName string, orderName string, params map[string]string) (*console.PayResult, error) {
	mainConfig, err := k.getMainConfig()
	if err != nil {
		return nil, err
	}
	currentConfig, err := k.w7respo.Get(currentConfigName)
	if err != nil {
		return nil, err
	}
	product, err := k.consoleSdkClient.PrepareProduct2()
	if err != nil {
		return nil, err
	}
	return createOrderByName(orderName, mainConfig, currentConfig, product.ProductId, params, k.consoleSdkClient)
}

func (k *K3kOrderApi) CreateBaseResourceOrder(baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {

	currentUq := baseResource.UnitQuantity
	if currentUq.IsEmpty() {
		return nil, fmt.Errorf("购买时长不能为空")
	}
	compute, err := user.GetOrderCompute()
	if err != nil {
		return nil, err
	}

	defaultBuyResource := compute.BuyResource.Clone()
	if baseResource.BuyResource.Less(defaultBuyResource) {
		return nil, fmt.Errorf("购买资源小于当前最低购买资源")
	}
	compute = compute.WithResource(baseResource.BuyResource).WithQuantity(currentUq) //会Clone一份
	conponCode := baseResource.CouponCode
	used := false
	if !compute.IsGiveInBaseBuyMode() {
		compute = k.applyCoupon(compute, conponCode, user)
		used = compute.IsCouponMatch()
	}

	price := compute.GetDiscountPrice(BASE_BUY)

	priceStr := price.String()

	hourstr := strconv.FormatFloat(currentUq.GetHours(), 'f', 2, 64)
	params := compute.ToReqParams()
	params["buymode"] = BASE_BUY
	params["price"] = priceStr
	params["hour"] = hourstr
	err = k.LockCoupon(conponCode, used)
	if err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, BASE_BUY)
	if err != nil {
		return nil, err
	}
	err = k.UsedCoupon(conponCode, used, result.OrderSn)
	if err != nil {
		slog.Error("used coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("used coupon code error")
	}
	return result, err
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
		compute = k.applyCouponToCvm(compute, conponCode, user)
		used = compute.IsCouponMatch()
	}
	params := compute.ToReqParams()
	params["buymode"] = BASE_BUY
	params["price"] = compute.GetDiscountPrice(BASE_BUY).String()
	params["hour"] = strconv.FormatFloat(currentUq.GetHours(), 'f', 2, 64)
	if err := k.LockCoupon(conponCode, used); err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrderByName(baseResource.BaseConfigName, user.Name, cvm.Name, params)
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

func (k *K3kOrderApi) CreateRenewOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {

	hourstr := strconv.FormatFloat(baseResource.GetHours(), 'f', 2, 64)
	compute, err := user.GetOrderCompute()
	if err != nil {
		return nil, err
	}
	compute = compute.WithQuantity(baseResource.UnitQuantity)
	conponCode := baseResource.CouponCode
	compute = k.applyCoupon(compute, conponCode, user)
	used := compute.IsCouponMatch()
	price := compute.GetDiscountPrice(RENEW_BUY)

	params := compute.ToReqParams()
	params["buymode"] = RENEW_BUY
	params["price"] = price.String()
	params["hour"] = hourstr
	err = k.LockCoupon(conponCode, used)
	if err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrder(baseResource.BaseConfigName, user, params, RENEW_BUY)
	if err != nil {
		return nil, err
	}
	err = k.UsedCoupon(conponCode, used, result.OrderSn)
	if err != nil {
		slog.Error("used coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("used coupon code error")
	}
	return result, err
}

func (k *K3kOrderApi) CreateRenewCvmOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {
	cvm, err := k.getCvm(user, baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return nil, err
	}
	if err := cvmOrder.CanRenewError(); err != nil {
		return nil, err
	}
	currentResource := currentCvmBuyResource(cvm)
	compute := types.NewK3kOrderCompute(currentResource, baseResource.UnitQuantity, user.GetCost(), nil)
	conponCode := baseResource.CouponCode
	compute = k.applyCouponToCvm(compute, conponCode, user)
	used := compute.IsCouponMatch()
	params := compute.ToReqParams()
	params["buymode"] = RENEW_BUY
	params["price"] = compute.GetDiscountPrice(RENEW_BUY).String()
	params["hour"] = strconv.FormatFloat(baseResource.GetHours(), 'f', 2, 64)
	if err := k.LockCoupon(conponCode, used); err != nil {
		slog.Error("lock coupon code error", "code", conponCode, "err", err)
		return nil, errors.New("lock coupon code error")
	}
	result, err := k.createOrderByName(baseResource.BaseConfigName, user.Name, cvm.Name, params)
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

// 扩容
func (k *K3kOrderApi) CreateExpandOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {

	err := baseResource.Valid()
	if err != nil {
		return nil, err
	}
	compute, err := user.GetOrderCompute()
	if err != nil {
		return nil, err
	}
	currentRs := compute.BuyResource
	if baseResource.BuyResource.Less(currentRs) {
		return nil, fmt.Errorf("扩容资源小于当前购买资源")
	}
	diff := baseResource.BuyResource.Sub(currentRs)
	err = diff.Valid()
	if err != nil {
		return nil, err
	}
	compute = compute.WithResource(diff)
	time2, err := user.GetExpireTime()
	if err != nil {
		return nil, err
	}
	if time2.IsZero() {
		return nil, fmt.Errorf("暂不支持单个资源购买")
	}
	if time2.Before(time.Now()) {
		return nil, fmt.Errorf("账户已过期，无法扩容")
	}
	sub := time2.Sub(time.Now())
	hour := sub.Hours()
	hourstr := decimal.NewFromInt32((int32(hour))).String()
	// months := decimal.NewFromFloat(hour).Div(decimal.NewFromInt32(30 * 24)) /// 24 / 30
	// strconv.FormatFloat(hour, 'f', 2, 64)
	buyRs := compute.BuyResource
	price, err := compute.GetExpandPrice(time2)
	if err != nil {
		return nil, err
	}
	params := make(map[string]string)
	params["buymode"] = EXPAND_BUY
	params["price"] = price.String()
	params["hour"] = hourstr
	params["cpu"] = strconv.FormatInt(int64(buyRs.Cpu), 10)
	params["memory"] = strconv.FormatInt(int64(buyRs.Memory), 10)
	params["storage"] = strconv.FormatInt(int64(buyRs.Storage), 10)
	params["bandwidth"] = strconv.FormatInt(int64(buyRs.Bandwidth), 10)
	return k.createOrder(baseResource.BaseConfigName, user, params, EXPAND_BUY)
}

func (k *K3kOrderApi) CreateExpandCvmOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {
	cvm, err := k.getCvm(user, baseResource.CvmName)
	if err != nil {
		return nil, err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return nil, err
	}
	if err := cvmOrder.CanExpandError(); err != nil {
		return nil, err
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
	result, err := k.createOrderByName(baseResource.BaseConfigName, user.Name, cvm.Name, params)
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
	return result, nil
}

func (k *K3kOrderApi) createOrder(baseConfigName string, user *types.K3kUser, params map[string]string, buyMode string) (*console.PayResult, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	baseConfigName = license.FounderSaName

	w7respo := k.w7respo
	w7config, err := w7respo.Get(baseConfigName)
	if err != nil {
		return nil, err
	}
	currentConfig, err := w7respo.Get(user.Name)
	if err != nil {
		return nil, err
	}
	// sdkClient, err := console.NewSdkClient(license)
	// if err != nil {
	// 	return nil, err
	// }
	product, err := k.consoleSdkClient.PrepareProduct2()
	// product, err := PrepareProduct(w7config)
	if err != nil {
		return nil, err
	}
	payResult, err := createOrder(user, w7config, currentConfig, product.ProductId, params, k.consoleSdkClient)
	if err != nil {
		return nil, err
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
		if buyMode == BASE_BUY {
			user.SetBaseOrder(payResult.OrderSn)
		}
		if buyMode == RENEW_BUY {
			user.SetRenewOrder(payResult.OrderSn)
		}
		if buyMode == EXPAND_BUY {
			user.SetExpandOrder(payResult.OrderSn)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !payResult.NeedPay {
		time.AfterFunc(time.Second*2, func() {
			k.NotifyOrder(user, payResult.OrderSn) //0延迟5秒，防止订单还未创建完成 k3kuser 可能还没创建完成，延迟5秒再通知
		})
	}

	return payResult, nil
}

func (k *K3kOrderApi) Refresh(user *types.K3kUser) error {
	baseOrderSn := user.GetBaseOrderSn()
	renewOrderSn := user.GetRenewOrderSn()
	expandOrderSn := user.GetExpandOrderSn()

	sns := []string{baseOrderSn, renewOrderSn, expandOrderSn}
	for _, sn := range sns {
		if sn == "" {
			continue
		}
		err := k.NotifyOrder(user, sn)
		if err != nil {
			slog.Warn("订单通知失败", "orderSn", sn, "error", err)
			return err
		}
	}
	return nil

}
func (k *K3kOrderApi) NotifyOrder(user *types.K3kUser, sn string) error {
	slog.Error("订单通知", "orderSn", sn)
	w7config, err := k.w7respo.Get(user.Name)
	if err != nil {
		return err
	}
	orderInfo, err := getOrderSn(user, w7config, sn)
	if err != nil {
		slog.Warn("获取订单信息失败", "orderSn", sn, "error", err)
	}

	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		user.SetOrderStatus(orderInfo)
		return nil
	})
	if err != nil {
		slog.Warn("更新订单状态失败", "orderSn", sn, "error", err)
	}
	return err
}

func (k *K3kOrderApi) NotifyCvmOrder(user *types.K3kUser, cvmName string, sn string) error {
	slog.Error("cvm订单通知", "orderSn", sn, "cvm", cvmName)
	cvm, err := k.getCvm(user, cvmName)
	if err != nil {
		return err
	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return err
	}
	w7config, err := k.w7respo.Get(user.Name)
	if err != nil {
		return err
	}
	orderInfo, err := getOrderSnByName(cvm.Name, w7config, sn)
	if err != nil {
		slog.Warn("获取cvm订单信息失败", "orderSn", sn, "error", err)
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		cvmOrder.SetOrderStatus(orderInfo)
		return nil
	})
	if err != nil {
		slog.Warn("更新cvm订单状态失败", "orderSn", sn, "error", err)
	}
	return err
}

// 模拟支付成功，用于测试
func (k *K3kOrderApi) MockNotifyPaidOrder(user *types.K3kUser, sn string) error {
	slog.Error("订单通知", "orderSn", sn)
	w7config, err := k.w7respo.Get(user.Name)
	if err != nil {
		return err
	}
	orderInfo, err := getOrderSn(user, w7config, sn)
	if err != nil {
		slog.Warn("获取订单信息失败", "orderSn", sn, "error", err)
	}
	orderInfo.OrderStatus = "paid"
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		user.SetOrderStatus(orderInfo)
		return nil
	})
	if err != nil {
		slog.Warn("更新订单状态失败", "orderSn", sn, "error", err)
	}
	return err
}

func (k *K3kOrderApi) FindLastPaidOrder(user *types.K3kUser) (*console.LastPaidOrder, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	baseConfigName := license.FounderSaName
	// w7respo := k.w7respo
	w7config, err := k.w7respo.Get(baseConfigName)
	if err != nil {
		return nil, err
	}
	return k.consoleSdkClient.FindLastPaidOrder(w7config.ClusterId, user.Name)
}

func (k *K3kOrderApi) FindLastPaidOrderByName(name string) (*console.LastPaidOrder, error) {
	w7config, err := k.getMainConfig()
	if err != nil {
		return nil, err
	}
	return k.consoleSdkClient.FindLastPaidOrder(w7config.ClusterId, name)
}

func (k *K3kOrderApi) FindLastReturnOrder(user *types.K3kUser) (*console.LastReturnOrder, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	baseConfigName := license.FounderSaName
	// w7respo := k.w7respo
	w7config, err := k.w7respo.Get(baseConfigName)
	if err != nil {
		return nil, err
	}
	return k.consoleSdkClient.FindLastReturnOrder(w7config.ClusterId, user.Name)
}

// 软事务 先记录下要更改的记录，然后标记处理完成
func (k *K3kOrderApi) ProcessReturnOrder(user *types.K3kUser) error {
	if !user.IsClusterUser() {
		return nil
	}

	if user.HasProcessReturnOrder() {
		returnOrder, err := user.GetLockReturnK3kOrder()
		if err != nil {
			return err
		}
		order, err := k.consoleSdkClient.FindK3kOrder(user.Name, returnOrder.OrderSn)
		if err != nil {
			return err
		}
		if order.ReturnAt == "" {
			_, err := k.consoleSdkClient.ReturnOrderFinish(user.Name, returnOrder.OrderSn)
			if err != nil {
				return err
			}
		}
		_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
			k.mu.Lock()
			defer k.mu.Unlock()
			user.ProcessReturnK3kOrder()
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

// 软事务 先记录下要更改的记录，然后标记处理完成
func (k *K3kOrderApi) ProcessReturnLastOrder(user *types.K3kUser, process bool) error {
	if !user.IsClusterUser() {
		return nil
	}
	returnOrder, err := k.FindLastReturnOrder(user)
	if err != nil {
		return err
	}
	slog.Error("处理return订单", "orderSn", returnOrder.K3kOrder.OrderSn)
	//先锁定数据
	if returnOrder.HasOrder {
		_, err := controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
			k.mu.Lock()
			defer k.mu.Unlock()
			user.LockReturnK3kOrder(returnOrder.K3kOrder) //锁定要处理的资源
			return nil
		})
		if err != nil {
			return err
		}
	}
	if process {
		return k.ProcessReturnOrder(user)
	}
	return nil
}

func (k *K3kOrderApi) CheckCanBuy(user *types.K3kUser) error {
	order, err := k.FindLastPaidOrder(user)
	if err != nil {
		return err
	}
	if !order.CanBuy {
		return errors.New(order.Error)
	}
	return nil
}

func (k *K3kOrderApi) CheckCanBuyCvm(cvmName string) error {
	order, err := k.FindLastPaidOrderByName(cvmName)
	if err != nil {
		return err
	}
	if !order.CanBuy {
		return errors.New(order.Error)
	}
	return nil
}

func createOrder(user *types.K3kUser, mainConfig *config.W7Config, currentConfig *config.W7Config, productId int32, params map[string]string, client *console.SdkClient) (order *console.PayResult, err error) {
	return createOrderByName(user.Name, mainConfig, currentConfig, productId, params, client)
}

func createOrderByName(orderName string, mainConfig *config.W7Config, currentConfig *config.W7Config, productId int32, params map[string]string, client *console.SdkClient) (order *console.PayResult, err error) {
	values := url.Values{}
	values.Set("productId", strconv.Itoa(int(productId)))
	values.Set("clusterId", mainConfig.ClusterId)
	values.Set("k3kName", orderName)
	values.Set("appid", client.License.AppId)
	for k, v := range params {
		values.Add(k, v)
	}
	// return client.CreatePanelOrder(values) //sdk 导致获取的用户id 是appid 站点的bbsuid
	apiClient := console.NewConsoleCdClient(currentConfig.ThirdpartyCDToken)
	return apiClient.CreatePanelOrder(values)

}
func CreateBaseResourceOrder(baseResource *types.BuyBaseResource, user *types.K3kUser) (*console.PayResult, error) {
	if baseResource.CvmName != "" {
		return CreateBaseResourceCvmOrder(baseResource, user)
	}
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

func CreateRenewOrder(baseResource *types.BuyRenewResource, user *types.K3kUser) (*console.PayResult, error) {
	if baseResource.CvmName != "" {
		return CreateRenewCvmOrder(baseResource, user)
	}
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

func CreateExpandOrder(baseResource *types.BuyExpandResource, user *types.K3kUser) (*console.PayResult, error) {
	if baseResource.CvmName != "" {
		return CreateExpandCvmOrder(baseResource, user)
	}
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

func PrepareProduct(w7config *config.W7Config) (*console.GoodsProduct, error) {
	apiClient := console.NewConsoleCdClient(w7config.ThirdpartyCDToken)
	return apiClient.PrepareProduct()
}

func Refresh(user *types.K3kUser) error {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	return orderApi.Refresh(user)
}

func NotifyOrder(user *types.K3kUser, sn string) error {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	return orderApi.NotifyOrder(user, sn)
}

func MockNotifyOrder(user *types.K3kUser, sn string) error {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	return orderApi.MockNotifyPaidOrder(user, sn)
}

func CheckCanBuy(user *types.K3kUser) error {
	sdk := k8s.NewK8sClient().Sdk
	orderApi, err := NewK3kOrderApi(sdk)
	if err != nil {
		return err
	}
	return orderApi.CheckCanBuy(user)
}

func getOrderSn(user *types.K3kUser, w7config *config.W7Config, orderSn string) (*console.OrderInfo, error) {
	return getOrderSnByName(user.Name, w7config, orderSn)
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
