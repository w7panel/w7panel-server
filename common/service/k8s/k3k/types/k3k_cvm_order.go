package types

import (
	"errors"
	"time"

	"github.com/w7panel/w7panel/common/service/console"
	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
)

type K3kCvmOrder struct {
	*v1alpha1.Cvm
	*k3kCvmOverSelling
	*k3kCvmTime
	cost *K3kCost
}

func Newk3kCvmOrder(cvm *v1alpha1.Cvm, overUser *k3kCvmOverSelling, userTime *k3kCvmTime, cost *K3kCost) *K3kCvmOrder {
	return &K3kCvmOrder{cvm, overUser, userTime, cost}
}

func copyCvmResource(resource *v1alpha1.CvmResource) *v1alpha1.CvmResource {
	if resource == nil {
		return nil
	}
	return &v1alpha1.CvmResource{
		CPU:       resource.CPU,
		Memory:    resource.Memory,
		Storage:   resource.Storage,
		Bandwidth: resource.Bandwidth,
	}
}

func addCvmResource(base *v1alpha1.CvmResource, cpu, memory, storage, bandwidth int64) *v1alpha1.CvmResource {
	if base == nil {
		base = &v1alpha1.CvmResource{}
	}
	resource := copyCvmResource(base)
	resource.CPU += cpu
	resource.Memory += memory
	resource.Storage += storage
	resource.Bandwidth += bandwidth
	return resource
}

func (u *K3kCvmOrder) setCapacityCheckPending() {
	u.Status.CapacityCheckState = "wait"
}

func (u *K3kCvmOrder) SetBaseOrder(orderSn string) {

	u.Spec.BaseOrder = &v1alpha1.CvmOrder{
		OrderSn: orderSn,
	}
	// u.Labels[W7_BASE_ORDER_SN] = orderSn
}

func (u *K3kCvmOrder) SetRenewOrder(orderSn string) {
	u.Spec.RenewOrder = &v1alpha1.CvmOrder{
		OrderSn: orderSn,
		Status:  W7_ORDER_WAIT,
	}
	// u.Labels[W7_RENEW_ORDER_SN] = orderSn
	// u.Labels[W7_RENEW_ORDER_STATUS] = W7_ORDER_WAIT //因为支付成功后会判断是否PAID，所以这里先设置为等待支付
}

func (u *K3kCvmOrder) SetExpandOrder(orderSn string) {
	// u.Labels[W7_EXPAND_ORDER_SN] = orderSn
	// u.Labels[W7_EXPAND_ORDER_STATUS] = W7_ORDER_WAIT
	u.Spec.ExpandOrder = &v1alpha1.CvmOrder{
		OrderSn: orderSn,
		Status:  W7_ORDER_WAIT,
	}
}

func (u *K3kCvmOrder) GetBaseOrderSn() string {
	if u.Spec.BaseOrder == nil {
		return ""
	}
	return u.Spec.BaseOrder.OrderSn
}

func (u *K3kCvmOrder) GetRenewOrderSn() string {
	if u.Spec.RenewOrder == nil {
		return ""
	}
	return u.Spec.RenewOrder.OrderSn
}

func (u *K3kCvmOrder) GetExpandOrderSn() string {
	if u.Spec.ExpandOrder == nil {
		return ""
	}
	return u.Spec.ExpandOrder.OrderSn
}

func (u *K3kCvmOrder) SetBaseOrderPaid(info *console.OrderInfo) {
	if u.Spec.BaseOrder == nil {
		return
	}
	if u.Spec.BaseOrder.OrderSn == info.OrderSn && u.Spec.BaseOrder.Status != W7_ORDER_PAID {
		// u.Labels[K3K_BUY_MODE] = "buy"

		// u.Labels[W7_BASE_ORDER_STATUS] = W7_ORDER_PAID
		u.Spec.BaseOrder.Status = W7_ORDER_PAID
		u.Spec.BaseOrder.Hour = int(info.GetHour())
		u.changeExpireTime(int(info.GetHour()))
		baseResource := BuyResource{
			Cpu:       info.Cpu,
			Memory:    info.Memory,
			Storage:   info.Storage,
			Bandwidth: info.Bandwidth,
		}
		u.Spec.BaseOrder.Resource = &v1alpha1.CvmResource{
			CPU:       baseResource.Cpu,
			Memory:    baseResource.Memory,
			Storage:   baseResource.Storage,
			Bandwidth: baseResource.Bandwidth,
		}
		u.Spec.PurchasedResource = copyCvmResource(u.Spec.BaseOrder.Resource)
		if u.Spec.ProvisionMode == "" {
			u.Spec.ProvisionMode = "order-required"
		}

		u.setCapacityCheckPending()

	}
}

func (u *K3kCvmOrder) SetRenewOrderPaid(info *console.OrderInfo) {
	if u.Spec.RenewOrder == nil {
		return
	}
	// u.Spec.RenewOrder = &v1alpha1.CvmOrder{
	// 	OrderSn: info.OrderSn,
	// 	Status:  W7_ORDER_WAIT,
	// }
	if u.Spec.RenewOrder.OrderSn == info.OrderSn && u.Spec.RenewOrder.Status != W7_ORDER_PAID {
		// u.Labels[K3K_BUY_MODE] = "renew"
		u.Spec.RenewOrder.Status = W7_ORDER_PAID
		u.Spec.RenewOrder.Hour = int(info.GetHour())
		u.changeExpireTime(int(info.GetHour()))
	}
}

func (u *K3kCvmOrder) SetExpandOrderPaid(info *console.OrderInfo) {
	if u.Spec.ExpandOrder == nil {
		return
	}

	if u.Spec.ExpandOrder.OrderSn == info.OrderSn && u.Spec.ExpandOrder.Status != W7_ORDER_PAID {
		// u.Labels[K3K_BUY_MODE] = "renew"
		u.Spec.ExpandOrder.Status = W7_ORDER_PAID
		u.Spec.ExpandOrder.Hour = int(info.GetHour())
		u.Spec.ExpandOrder.Resource = &v1alpha1.CvmResource{
			CPU:       info.Cpu,
			Memory:    info.Memory,
			Storage:   info.Storage,
			Bandwidth: info.Bandwidth,
		}
		base := u.Spec.PurchasedResource
		u.Spec.PurchasedResource = addCvmResource(base, info.Cpu, info.Memory, info.Storage, info.Bandwidth)
		if u.Spec.ProvisionMode == "" {
			u.Spec.ProvisionMode = "order-required"
		}
		u.setCapacityCheckPending()
	}
	// if u.Labels[W7_EXPAND_ORDER_SN] == info.OrderSn && u.Labels[W7_EXPAND_ORDER_STATUS] != W7_ORDER_PAID {
	// 	// u.Labels[K3K_BUY_MODE] = "renew"
	// 	u.Labels[W7_EXPAND_ORDER_STATUS] = W7_ORDER_PAID
	// 	// u.changeExpireTime(hour)
	// 	u.Annotations[W7_OVER_RESOURCE] = overselling.OrderInfoToResource(info).JsonString()
	// 	u.Labels[W7_OVER_MODE] = "wait"
	// }
}

func (u *K3kCvmOrder) SetOrderStatus(info *console.OrderInfo) {
	if info.OrderStatus != "paid" {
		return
	}
	switch info.BuyMode {
	case "base":
		u.SetBaseOrderPaid(info)
		break
	case "renew":
		u.SetRenewOrderPaid(info)
		break
	case "expand":
		u.SetExpandOrderPaid(info)
		break
	}
}

func (u *K3kCvmOrder) NeedCreateOrder() bool {
	pass, ok := u.Labels[W7_BASE_ORDER_PASS]
	if ok {
		return pass == "false"
	}
	if u.NeedBuyResource() {
		if u.Spec.BaseOrder == nil {
			return true
		}
		if u.Spec.BaseOrder.Status == W7_ORDER_PAID {
			return false
		}
		return true
	}
	return false
}

// 是否可以续费

// 到期后必须续费，否则无法使用
func (u *K3kCvmOrder) NeedRenew() bool {
	if u.NeedBuyResource() {
		expireTime, err := u.GetExpireTime()
		if err != nil {
			return false
		}
		if expireTime.Before(time.Now()) { //3天内可以续费
			// if expireTime.Before(time.Now().Add(-time.Hour * 72)) { //3天内可以续费
			return true
		}
	}
	// if err := u.CanRenewError(); err != nil {
	// 	expireTime, err := u.GetExpireTime()
	// 	if err != nil {
	// 		return false
	// 	}
	// 	if expireTime.Before(time.Now()) { //3天内可以续费
	// 		// if expireTime.Before(time.Now().Add(-time.Hour * 72)) { //3天内可以续费
	// 		return true
	// 	}
	// }
	return false
}

func (u *K3kCvmOrder) CanCreateBaseOrderError() error {
	pass, ok := u.Labels[W7_BASE_ORDER_PASS]
	if ok {
		if pass == "false" {
			return nil
		}
	}
	if u.NeedBuyResource() {
		if u.Spec.BaseOrder == nil {
			return nil
		}
		if u.Spec.BaseOrder.Status == W7_ORDER_PAID {
			return errors.New("已经购买基础资源，无法重复购买")
		}
		return nil
	}
	return errors.New("当前用户未配置费用套餐，无法购买")
}

func (u *K3kCvmOrder) CanRenewError() error {
	if u.NeedBuyResource() {
		_, err := u.GetExpireTime() // 如果没有过期时间，则不需要续费
		if err != nil {
			return errors.New("未购买基础资源，无需购买")
		}
		return nil
	}
	return errors.New("当前用户未配置费用套餐，无法购买")
}

func (u *K3kCvmOrder) CanExpandError() error {
	if !u.IsOverSellingSuccess() {
		return errors.New("超额检查失败，无法扩容")
	}
	if u.NeedBuyResource() {
		extime, err := u.GetExpireTime() // 如果没有过期时间，则不需要续费
		if err != nil {
			return errors.New("未购买基础资源，无法扩容")
		}
		ok := extime.After(time.Now())
		if ok {
			return nil
		}
		return errors.New("基础资源已过期，无法扩容")
	}
	return errors.New("当前用户未配置费用套餐，无法购买")
}

func (u *K3kCvmOrder) NeedBuyResource() bool {
	if u.cost != nil {
		return true
	}
	return false
}

func (u *K3kCvmOrder) HasProcessReturnOrder() bool {
	data, ok := u.Annotations[W7_RETURN_ORDER_INFO]
	if ok && data != "" {
		return true
	}
	return false
}
