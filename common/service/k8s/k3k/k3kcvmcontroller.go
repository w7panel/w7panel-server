package k3k

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	k3kv1 "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	K3kCvmFinalizerName = "cvm.k3k.io/finalizer"
	K3kCvmNameLabel     = "w7.cc/cvm-name"
	K3kCvmNamespaceAnno = "w7.cc/cvm-namespace"
)

type K3kCvmController struct {
	client.Client
	Scheme *runtime.Scheme
	Sdk    *k8s.Sdk
}

func setupCvmController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	r := &K3kCvmController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cvmv1alpha1.Cvm{}).
		Owns(&k3kv1.Cluster{}).
		Complete(r)
}

func (r *K3kCvmController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("K3kCvm reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
			"namespace", req.Namespace,
		)
	}
	return result, err
}

func (r *K3kCvmController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in K3kCvm Handle", "panic", r)
		}
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling Cvm", "namespace", req.Namespace, "name", req.Name)

	cvm := &cvmv1alpha1.Cvm{}
	if err := r.Get(ctx, req.NamespacedName, cvm); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Cvm")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if !cvm.DeletionTimestamp.IsZero() {
		logger.Info("Cvm is being deleted", "namespace", req.Namespace, "name", req.Name)
		return r.handleDeletion(ctx, cvm)
	}

	if !controllerutil.ContainsFinalizer(cvm, K3kCvmFinalizerName) {
		logger.Info("Adding finalizer", "namespace", req.Namespace, "name", req.Name)
		controllerutil.AddFinalizer(cvm, K3kCvmFinalizerName)
		if err := r.Update(ctx, cvm); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	cluster, err := r.createOrUpdateCluster(ctx, cvm)
	if err != nil {
		logger.Error(err, "Failed to create/update k3k Cluster")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	_ = cluster

	return ctrl.Result{}, nil
}

func (r *K3kCvmController) createOrUpdateCluster(ctx context.Context, cvm *cvmv1alpha1.Cvm) (*k3kv1.Cluster, error) {
	cluster := &k3kv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getClusterName(cvm),
			Namespace: r.getClusterNamespace(cvm),
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cluster, func() error {
		if cluster.Labels == nil {
			cluster.Labels = make(map[string]string)
		}
		if cluster.Annotations == nil {
			cluster.Annotations = make(map[string]string)
		}
		cluster.Labels["cvm-uid"] = string(cvm.UID)
		cluster.Labels[K3kCvmNameLabel] = cvm.Name
		cluster.Annotations[K3kCvmNamespaceAnno] = cvm.Namespace

		cluster.Spec = r.toClusterSpec(cvm)
		return controllerutil.SetControllerReference(cvm, cluster, r.Scheme)
	})
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (r *K3kCvmController) toClusterSpec(cvm *cvmv1alpha1.Cvm) k3kv1.ClusterSpec {
	servers := int32(1)
	spec := k3kv1.ClusterSpec{
		Servers: &servers,
	}

	if cvm.Spec.StorageClassName != "" {
		spec.Persistence.StorageClassName = &cvm.Spec.StorageClassName
	}
	if cvm.Spec.Resource.Storage > 0 {
		spec.Persistence.StorageRequestSize = fmt.Sprintf("%dGi", cvm.Spec.Resource.Storage)
	}

	return spec
}

func (r *K3kCvmController) handleDeletion(ctx context.Context, cvm *cvmv1alpha1.Cvm) (ctrl.Result, error) {
	slog.Info("Handling Cvm deletion", "name", cvm.Name, "namespace", cvm.Namespace)

	cluster := &k3kv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getClusterName(cvm),
			Namespace: r.getClusterNamespace(cvm),
		},
	}
	if err := r.Delete(ctx, cluster); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if err := r.ensureClusterDeleted(ctx, cvm); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	controllerutil.RemoveFinalizer(cvm, K3kCvmFinalizerName)
	if err := r.Update(ctx, cvm); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

func (r *K3kCvmController) getClusterName(cvm *cvmv1alpha1.Cvm) string {
	if cvm.Annotations != nil {
		if name := cvm.Annotations[k3ktypes.K3K_NAME]; name != "" {
			return name
		}
	}
	return cvm.Name
}

func (r *K3kCvmController) getClusterNamespace(cvm *cvmv1alpha1.Cvm) string {
	if cvm.Annotations != nil {
		if ns := cvm.Annotations[k3ktypes.K3K_NAMESPACE]; ns != "" {
			return ns
		}
	}
	return "default"
}

func (r *K3kCvmController) ensureClusterDeleted(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {
	cluster := &k3kv1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      r.getClusterName(cvm),
		Namespace: r.getClusterNamespace(cvm),
	}, cluster)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
