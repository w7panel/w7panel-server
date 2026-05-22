package k3k

import (
	"context"
	"time"

	cvmv1alpha1 "github.com/w7panel/w7panel-ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type K3kJobController struct {
	client.Client
	Scheme *runtime.Scheme
	Sdk    *k8s.Sdk
}

func setupJobController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	r := &K3kJobController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		Complete(r)
}

// Reconcile for Job controller
func (r *K3kJobController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Job", "namespace", req.Namespace, "name", req.Name)

	// Fetch the Job instance
	job := &batchv1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Job")
			return ctrl.Result{}, err
		}
		// Job was deleted
		return ctrl.Result{}, nil
	}

	// Handle Job
	isK3kjob := job.Labels["k3k-job"] == "true"
	if isK3kjob {
		cvmName := job.Labels["k3k-cvm-name"]
		if cvmName == "" {
			logger.Info("k3k-sa label is empty")
			return ctrl.Result{}, nil
		}
		cvm := &cvmv1alpha1.Cvm{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: job.Labels["k3k-namespace"], Name: cvmName}, cvm); err != nil {
			logger.Error(err, "Failed to get ServiceAccount")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if job.Labels[k3ktypes.W7_WH_MODE] == "true" {
			_, err := controllerutil.CreateOrPatch(ctx, r.Client, cvm, func() error {
				cvm.Status.RescuePhase = "running"
				if job.Status.Succeeded > 0 {
					cvm.Status.RescuePhase = "success"
				}
				if job.Status.Failed > 0 {
					cvm.Status.RescuePhase = "failed"
				}
				return nil
			})
			if err != nil {
				logger.Error(err, "Failed to update weihu job status")
				return ctrl.Result{RequeueAfter: time.Minute}, err
			}
			return ctrl.Result{}, nil
		}

	}

	return ctrl.Result{}, nil
}
