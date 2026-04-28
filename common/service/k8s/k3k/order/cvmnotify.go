package order

import (
	"context"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (k *K3kOrderApi) NotifyCvmOrder(user *types.K3kUser, cvmName string, sn string) error {
	slog.Error("cvm订单通知", "orderSn", sn, "cvm", cvmName)

	w7config, err := k.w7respo.Get(user.Name)
	if err != nil {
		return err
	}
	orderInfo, err := getOrderSnByName(user.GetK3kName(), w7config, sn)
	if err != nil {
		slog.Warn("获取cvm订单信息失败", "orderSn", sn, "error", err)
	}
	return doNotify(orderInfo, k, user)
}

func doNotify(orderInfo *console.OrderInfo, k *K3kOrderApi, user *types.K3kUser) error {
	cvmName := orderInfo.CvmName
	var crdOrder *cvmv1alpha1.CvmConsoleOrder
	if orderInfo.BuyMode == BASE_BUY { //只有基础购买会创建crd订单
		consoleOrder, err := k.getCvmConsoleOrder(user, orderInfo.OrderSn)
		if err != nil {
			return err
		}
		crdOrder = consoleOrder
	}
	if crdOrder != nil {
		_, err := controllerutil.CreateOrPatch(context.TODO(), k.client, crdOrder, func() error {
			crdOrder.Spec.Order = &cvmv1alpha1.CvmOrder{
				OrderSn: orderInfo.OrderSn,
				Status:  orderInfo.OrderStatus,
				Resource: &cvmv1alpha1.CvmResource{
					Bandwidth: orderInfo.Bandwidth,
					CPU:       orderInfo.Cpu,
					Memory:    orderInfo.Memory,
					Storage:   orderInfo.Storage,
				},
				Hour: int(orderInfo.GetHour()),
			}
			crdOrder.Spec.CvmName = cvmName
			return nil
		})
		if err != nil {
			slog.Warn("更新cvm订单状态失败", "orderSn", orderInfo.OrderSn, "error", err)
			return err
		}

	}

	cvm, err := k.getCvm(user, cvmName)
	if err != nil {
		if errors.IsNotFound(err) {
			cvm = &cvmv1alpha1.Cvm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crdOrder.Spec.CvmName,
					Namespace: user.GetK3kNamespace(),
				},
			}
		} else {
			return err
		}

	}
	cvmOrder, err := newCvmOrder(cvm, user)
	if err != nil {
		return err
	}
	_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		cvmOrder.SetOrderStatus(orderInfo)
		return nil
	})
	if err != nil {
		slog.Warn("更新cvm订单状态失败", "orderSn", orderInfo.OrderSn, "error", err)
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
	return doNotify(orderInfo, k, user)
}
