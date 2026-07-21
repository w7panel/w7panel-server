package permission

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SetupPermissionController sets up the Permission controller with the manager.
func SetupPermissionController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	if err := configv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	r := &PermissionController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&configv1alpha1.Permission{}).
		Complete(r)
}

// PermissionController reconciles Permission RBAC resources.
type PermissionController struct {
	client.Client
	Scheme *runtime.Scheme
	*k8s.Sdk
}

func (r *PermissionController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("Permission reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
		)
	}
	return result, err
}

func (r *PermissionController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("Recovered from panic in Permission Handle", "panic", rec)
		}
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling Permission", "name", req.Name)

	permission := &configv1alpha1.Permission{}
	if err := r.Get(ctx, req.NamespacedName, permission); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Permission")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if !permission.DeletionTimestamp.IsZero() {
		logger.Info("Permission is being deleted", "name", req.Name)
		return ctrl.Result{}, nil
	}

	if err := SyncPermissionAccount(ctx, r.Sdk, permission); err != nil {
		logger.Error(err, "Failed to sync Permission RBAC resources")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if IsBuiltin(permission) && permission.Name == NormalPermissionName {
		if err := r.ensureMetricsReaderAccess(ctx, permission.Name); err != nil {
			logger.Error(err, "Failed to bind metrics reader access")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *PermissionController) ensureMetricsReaderAccess(ctx context.Context, username string) error {
	if r.Client == nil {
		return fmt.Errorf("controller-runtime client is not configured")
	}

	const clusterRoleName = "ckm-metrics-reader"
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: username + "-" + clusterRoleName,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, clusterRoleBinding, func() error {
		clusterRoleBinding.Subjects = []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      username,
			Namespace: "default",
		}}
		clusterRoleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		}
		return nil
	})
	return err
}
