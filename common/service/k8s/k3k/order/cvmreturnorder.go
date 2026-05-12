package order

import (
	"context"
	"log/slog"

	cvmv1alpha1 "cnb.cool/i0358/ai-cvm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/console"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (k *K3kOrderApi) FindK3kOrder(userName string, orderSn string) (*console.K3kOrder, error) {
	return k.consoleSdkClient.FindK3kOrder(userName, orderSn)
}

func (k *K3kOrderApi) ReturnOrderFinish(userName string, orderSn string) (*console.LastReturnOrder, error) {
	return k.consoleSdkClient.ReturnOrderFinish(userName, orderSn)
}

// 软事务 先记录下要更改的记录，然后标记处理完成
func (k *K3kOrderApi) ProcessReturnOrder(cvm *cvmv1alpha1.Cvm) error {

	if cvm.HasReturnNoProcessOrder() {
		returnOrder := cvm.GetReturnNoProcessOrder()

		order, err := k.consoleSdkClient.FindK3kOrder(cvm.GetK3kName(), returnOrder.OrderSn)
		if err != nil {
			return err
		}
		if order.ReturnAt == "" {
			_, err := k.consoleSdkClient.ReturnOrderFinish(cvm.GetK3kName(), returnOrder.OrderSn)
			if err != nil {
				return err
			}
		}
		_, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, cvm, func() error {
			k.mu.Lock()
			defer k.mu.Unlock()
			cvm.ProcessReturnOrder(returnOrder.OrderSn)
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
func (k *K3kOrderApi) LockReturnLastOrder(ctx context.Context, cvm *cvmv1alpha1.Cvm, process bool) error {

	returnOrder, err := k.FindLastReturnCvmOrder(cvm)
	if err != nil {
		return err
	}
	if !returnOrder.HasOrder {
		slog.Error("cvm has no return order", "name", cvm.Name)
		return nil
	}
	cvmOrder := &cvmv1alpha1.CvmOrder{
		OrderSn: returnOrder.K3kOrder.OrderSn,
		Status:  returnOrder.K3kOrder.OrderStatus,
		Resource: &cvmv1alpha1.CvmResource{
			CPU:       returnOrder.K3kOrder.Cpu,
			Memory:    returnOrder.K3kOrder.Memory,
			Storage:   returnOrder.K3kOrder.Storage,
			Bandwidth: returnOrder.K3kOrder.Bandwidth,
		},
		BuyMode:      returnOrder.K3kOrder.BuyMode,
		Hour:         int(returnOrder.K3kOrder.GetHour()),
		ReturnFinish: false,
	}

	_, err = controllerutil.CreateOrPatch(ctx, k.client, cvm, func() error {
		if cvm.Spec.ReturnOrders == nil {
			cvm.Spec.ReturnOrders = make(map[string]*cvmv1alpha1.CvmOrder)
		}
		cvm.Spec.ReturnOrders[cvmOrder.OrderSn] = cvmOrder
		return nil
	})
	if err != nil {
		slog.Error("create or patch cvm order err", "err", err)
		return err
	}

	if process {
		return k.ProcessReturnOrder(cvm)
	}
	return nil
}
