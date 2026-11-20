package higress

import (
	"context"
	"log/slog"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	higressapinetworkingv1 "github.com/w7panel/w7panel/common/service/k8s/higress/api/networking/v1"
	higressnetworkingv1 "github.com/w7panel/w7panel/common/service/k8s/higress/client/pkg/apis/networking/v1"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func SetupManager(mgr ctrl.Manager, sdk *k8s.Sdk) error {

	reconciler := &McpBridgeReconciler{
		Client: mgr.GetClient(),
		sdk:    sdk,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		slog.Error("failed to setup controller", "error", err)
		return err
	}
	return nil
}

// McpBridgeReconciler 处理McpBridge资源的协调
type McpBridgeReconciler struct {
	client.Client
	sdk *k8s.Sdk
}

// Reconcile 处理McpBridge资源的变化
func (r *McpBridgeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	slog.Info("reconciling McpBridge", "name", req.Name, "namespace", req.Namespace)

	// 如果是default McpBridge，则跳过处理
	if req.Name == "default" {
		return reconcile.Result{}, nil
	}

	if helper.IsK3kShared() {
		error := k3k.SyncMcpBridgeHttp(k3k.NewSyncObject(req.Name, req.Namespace))
		if error != nil {
			slog.Error("failed to sync mcp bridge http", "error", error)
			return reconcile.Result{RequeueAfter: 10 * time.Second}, error
		}
		return reconcile.Result{}, nil
	}

	// 获取所有非default的McpBridge资源
	var mcpBridgeList higressnetworkingv1.McpBridgeList
	if err := r.List(ctx, &mcpBridgeList); err != nil {
		slog.Error("failed to list mcp bridges", "error", err)
		return reconcile.Result{}, err
	}

	// 合并所有非default McpBridge的配置
	var config map[string]*higressapinetworkingv1.RegistryConfig = make(map[string]*higressapinetworkingv1.RegistryConfig)
	for _, bridge := range mcpBridgeList.Items {
		if bridge.Name == "default" {
			continue
		}

		for _, registry := range bridge.Spec.Registries {
			config[registry.Name] = registry
		}
	}
	defaultConfig := []*higressapinetworkingv1.RegistryConfig{}
	var defaultMcp = &higressnetworkingv1.McpBridge{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "higress-system",
		},
		Spec: higressapinetworkingv1.McpBridge{
			Registries: defaultConfig,
		},
	}
	err := r.Get(ctx, types.NamespacedName{Namespace: "higress-system", Name: "default"}, defaultMcp)
	update := true
	if err != nil {
		if apierrors.IsNotFound(err) {
			update = false
		}
	}

	// 将所有配置添加到default McpBridge
	for _, registry := range config {
		defaultConfig = append(defaultConfig, registry)
	}
	newMap := defaultMcp.DeepCopy()
	newMap.Spec.Registries = defaultConfig
	if update {
		if err := r.Update(ctx, newMap); err != nil {
			slog.Error("failed to update mcp bridge", "error", err)
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
	} else {
		if err := r.Create(ctx, newMap); err != nil {
			slog.Error("failed to create mcp bridge", "error", err)
			return reconcile.Result{RequeueAfter: time.Minute}, err
		}
	}

	return reconcile.Result{}, nil
}

// SetupWithManager 将控制器注册到Manager
func (r *McpBridgeReconciler) SetupWithManager(mgr manager.Manager) error {
	// 创建过滤器，排除名为"default"的McpBridge资源
	defaultFilter := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetName() != "default"
	})

	// 设置控制器
	return ctrl.NewControllerManagedBy(mgr).
		For(&higressnetworkingv1.McpBridge{}).
		WithEventFilter(defaultFilter).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
