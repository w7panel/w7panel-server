package user

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// 兼容旧的service account Finalizer
func SetupServiceAccountController(mgr ctrl.Manager, sdk *k8s.Sdk) error {

	client := mgr.GetClient()
	r := &ServiceAccountController{
		Client: client,
		Scheme: mgr.GetScheme(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ServiceAccount{}).
		Complete(r)
}

// K3kServiceAccountController reconciles ServiceAccount objects
type ServiceAccountController struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ServiceAccountController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()

		// 结构化日志记录
		slog.Error("详细错误信息",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err))
		// slog.Error("result", "err", err, "result", result)
	}
	return result, err

}

// Reconcile for ServiceAccount controller
func (r *ServiceAccountController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in Handle", "panic", r)
		}
	}()
	logger := log.FromContext(ctx)
	logger.Info("Reconciling ServiceAccount", "namespace", req.Namespace, "name", req.Name)
	// slog.Error("start sa", "uname", req.Name)
	// Fetch the ServiceAccount instance
	sa := &corev1.ServiceAccount{}
	if err := r.Get(ctx, req.NamespacedName, sa); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get ServiceAccount")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		// ServiceAccount was deleted and we don't have finalizer, nothing to do
		return ctrl.Result{}, nil
	}
	// if sa.Name != "hello" {
	// 	return ctrl.Result{}, nil
	// }
	// Check if the ServiceAccount is being deleted
	if !sa.DeletionTimestamp.IsZero() {
		logger.Info("ServiceAccount is being deleted", "namespace", req.Namespace, "name", req.Name)
		slog.Debug("sa is deleting", "uname", req.Name)
		// finaly := []string{""}
		ok := controllerutil.RemoveFinalizer(sa, "k3k.sa/finalizer")
		if ok {
			err := r.Update(ctx, sa)
			if err != nil {
				logger.Error(err, "Failed to remove finalizer")
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}
	}

	return ctrl.Result{}, nil
}
