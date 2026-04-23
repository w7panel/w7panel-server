package k3k

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/sa"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func setupServiceAccountController(mgr ctrl.Manager, sdk *k8s.Sdk) error {

	client := mgr.GetClient()
	k3kClient := k3ktypes.NewK3kClient(client)
	limitClient := sa.NewLimitRangeClient(client)
	storage := sa.NewStorage(client)
	r := &K3kServiceAccountController{
		Client:      client,
		Scheme:      mgr.GetScheme(),
		k3kClient:   k3kClient,
		rolebinding: sa.NewRoleBinding(client),
		deleteRc:    sa.NewDeleteResource(client, k3kClient, limitClient),
		limitClient: limitClient,
		storage:     storage,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ServiceAccount{}).
		Complete(r)
}

// K3kServiceAccountController reconciles ServiceAccount objects
type K3kServiceAccountController struct {
	client.Client
	Scheme      *runtime.Scheme
	k3kClient   *k3ktypes.K3kClient
	rolebinding *sa.RoleBinding
	deleteRc    *sa.DeleteResource
	limitClient *sa.Limitrangeclient
	storage     *sa.Storage
}

func (r *K3kServiceAccountController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
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
func (r *K3kServiceAccountController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

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
	k3kUser := k3ktypes.NewK3kUser(sa)
	// Check if the ServiceAccount is being deleted
	if !sa.DeletionTimestamp.IsZero() {
		logger.Info("ServiceAccount is being deleted", "namespace", req.Namespace, "name", req.Name)
		slog.Debug("sa is deleting", "uname", req.Name)
		return r.deleteRc.HandleDeletion(ctx, sa, k3kUser)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(sa, k3ktypes.K3kFinalizerName) {
		logger.Info("Adding finalizer", "namespace", req.Namespace, "name", req.Name)
		controllerutil.AddFinalizer(sa, k3ktypes.K3kFinalizerName)
		if err := r.Update(ctx, sa); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		// Requeue to continue processing after finalizer is added
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	k3ktypes.SetSaVersion(sa.Name, sa.Annotations[k3ktypes.K3K_LOCK_VERSION])

	if k3kUser.IsNormalUser() {
		// 创建角色 需要job 查看权限
		err := r.rolebinding.CreateNormalUserRoleBinding(ctx, sa, helper.ServiceAccountName())
		if err != nil {
			logger.Error(err, "Failed to create normal user role binding")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}
	// if !k3kUser.IsClusterUser() {
	// 	// Not our ServiceAccount, ignore it
	// 	return ctrl.Result{}, nil
	// }

	// 处理资源回收阶段
	if err := r.deleteRc.HandleResourceRecycleStatus(ctx, sa, k3kUser); err != nil {
		logger.Error(err, "Failed to handle resource recycle status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: k3kUser.GetK3kNamespace(),
			Labels: map[string]string{
				"policy.k3k.io/policy-name": k3kUser.GetClusterPolicy(),
			},
		},
	}
	_, err := controllerutil.CreateOrPatch(ctx, r.Client, namespace, func() error {
		namespace.Labels = map[string]string{
			"policy.k3k.io/policy-name": k3kUser.GetClusterPolicy(),
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to create namespace")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	err = r.rolebinding.CreateRole(ctx, sa, k3kUser.GetK3kNamespace())
	if err != nil {
		logger.Error(err, "Failed to create role")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	// 创建 registries ConfigMap
	// err = r.createRegistriesConfigMap(ctx, k3kUser)
	// if err != nil {
	// 	logger.Error(err, "Failed to create registries ConfigMap")
	// 	return ctrl.Result{RequeueAfter: time.Second * 10}, err
	// }\
	if true {
		return ctrl.Result{}, nil
	}

	if !k3kUser.IsClusterReady() {
		slog.Error("cluster not ready", "uname", k3kUser.GetName())
		return ctrl.Result{}, nil
	}

	if k3kUser.IsWeihu() {
		jobName := k3kUser.GenerateWeihuJobName()
		if !k3kUser.HasWeihuJob() {
			_, err = controllerutil.CreateOrPatch(ctx, r.Client, k3kUser.ServiceAccount, func() error {
				k3kUser.SetWeihuJobName(jobName)
				return nil
			})
			if err != nil {
				slog.Error("failed to update weihu job2", "err", err)
				return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
			}
		}
		if k3kUser.HasWeihuJob() {
			job := types.ToK3kWeihJob(k3kUser)
			err := r.Client.Create(ctx, job)
			if err != nil {
				if k8serrors.IsAlreadyExists(err) {
					slog.Error("failed to create weihu job", "err", err)
					return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
				}
			}
		}

	} else {
		jobName := k3kUser.GetWeihuJobName()
		if jobName != "" {
			job := types.ToK3kWeihJob(k3kUser)
			err := r.Client.Delete(ctx, job)
			if err != nil {
				if !k8serrors.IsNotFound(err) {
					slog.Error("failed to delete weihu job", "err", err)
					return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
				}
			}
		}
	}

	if k3kUser.HasExpireTime() {
		return ctrl.Result{RequeueAfter: time.Minute * 30}, nil
	}

	return ctrl.Result{}, nil
}
