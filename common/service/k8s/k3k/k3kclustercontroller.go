package k3k

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"runtime/debug"
	"time"

	k3kv1 "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type K3kCvmClusterController struct {
	client.Client
	Scheme *runtime.Scheme
	Sdk    *k8s.Sdk
}

func setupCvmClusterController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	r := &K3kCvmClusterController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&k3kv1.Cluster{}).
		Complete(r)
}

func (r *K3kCvmClusterController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("K3kCvmCluster reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
			"namespace", req.Namespace,
		)
	}
	return result, err
}

func (r *K3kCvmClusterController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Cluster", "namespace", req.Namespace, "name", req.Name)

	cluster := &k3kv1.Cluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Cluster")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	cvmName := cluster.Labels[K3kCvmNameLabel]
	if cvmName == "" {
		return ctrl.Result{}, nil
	}

	cvmNamespace := cluster.Annotations[K3kCvmNamespaceAnno]
	cvm := &cvmv1alpha1.Cvm{}
	if err := r.Get(ctx, client.ObjectKey{Name: cvmName, Namespace: cvmNamespace}, cvm); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Cvm")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if err := r.updateCvmStatus(ctx, cvm, cluster); err != nil {
		logger.Error(err, "Failed to sync Cluster status to Cvm")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

func (r *K3kCvmClusterController) updateCvmStatus(ctx context.Context, cvm *cvmv1alpha1.Cvm, cluster *k3kv1.Cluster) error {
	newStatus := cvmv1alpha1.CvmStatus{
		Phase:         string(cluster.Status.Phase),
		Conditions:    append([]metav1.Condition(nil), cluster.Status.Conditions...),
		ReadyReplicas: 0,
	}

	if cluster.Status.Phase == k3kv1.ClusterReady {
		newStatus.ReadyReplicas = 1
	}
	if newStatus.Phase == "" {
		newStatus.Phase = "Pending"
	}

	if reflect.DeepEqual(cvm.Status, newStatus) {
		return nil
	}

	cvm.Status = newStatus
	if err := r.Update(ctx, cvm); err != nil {
		return err
	}

	slog.Info("Synced Cluster status to Cvm",
		"cvm", cvm.Name,
		"cluster", cluster.Name,
		"clusterNamespace", cluster.Namespace,
		"phase", newStatus.Phase,
		"readyReplicas", newStatus.ReadyReplicas,
	)

	return nil
}
