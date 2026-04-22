package order

import (
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

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

func (k *K3kOrderApi) MockNotifyPaidOrderCvm(user *types.K3kUser, sn string) error {
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
	cvm, err := k.getCvm(user, orderInfo.CvmName)
	if err != nil {
		return err
	}
	cvmClone := cvm.DeepCopy()
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvmClone, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		cvmOrder, err := newCvmOrder(cvmClone, user)
		if err != nil {
			return err
		}
		cvmOrder.SetOrderStatus(orderInfo)
		return nil
	})
	if err != nil {
		slog.Warn("更新订单状态失败", "orderSn", sn, "error", err)
	}
	return err
}
