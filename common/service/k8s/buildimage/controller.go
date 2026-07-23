package buildimage

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	buildimagev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/buildimage/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	BuildImageFinalizerName = "buildimage.k3k.io/finalizer"
)

// SetupBuildImageController sets up the BuildImage controller with the manager
func SetupBuildImageController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	k8sClient := mgr.GetClient()
	r := &BuildImageController{
		Client: k8sClient,
		Scheme: mgr.GetScheme(),
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&buildimagev1alpha1.BuildImage{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// BuildImageController reconciles BuildImage objects
type BuildImageController struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *BuildImageController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("BuildImage reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
			"namespace", req.Namespace,
		)
	}
	return result, err
}

func (r *BuildImageController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in BuildImage Handle", "panic", r)
		}
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling BuildImage", "namespace", req.Namespace, "name", req.Name)

	// Fetch the BuildImage instance
	buildImage := &buildimagev1alpha1.BuildImage{}
	if err := r.Get(ctx, req.NamespacedName, buildImage); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get BuildImage")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	// Check if the BuildImage is being deleted
	if !buildImage.DeletionTimestamp.IsZero() {
		logger.Info("BuildImage is being deleted", "namespace", req.Namespace, "name", req.Name)
		return r.handleDeletion(ctx, buildImage)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(buildImage, BuildImageFinalizerName) {
		logger.Info("Adding finalizer", "namespace", req.Namespace, "name", req.Name)
		controllerutil.AddFinalizer(buildImage, BuildImageFinalizerName)
		if err := r.Update(ctx, buildImage); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}

	// Convert spec to internal type
	spec := BuildImageSpec{
		BuildImageSpec: &buildImage.Spec,
	}

	// Create or update the build job
	job, err := r.createOrUpdateBuildJob(ctx, buildImage, &spec)
	if err != nil {
		logger.Error(err, "Failed to create/update build job")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update BuildImage status based on job status
	if err := r.updateBuildImageStatus(ctx, buildImage, job); err != nil {
		logger.Error(err, "Failed to update BuildImage status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

func (r *BuildImageController) createOrUpdateBuildJob(ctx context.Context, buildImage *buildimagev1alpha1.BuildImage, spec *BuildImageSpec) (*batchv1.Job, error) {
	job, err := toBuildJob(ctx, &BuildImageSpec{BuildImageSpec: &buildImage.Spec})
	if err != nil {
		return nil, err
	}
	job.Labels["build-image-uid"] = string(buildImage.UID)
	// job.Labels["w7.cc/build-image"] = "true"

	// Set controller reference
	if err := ctrl.SetControllerReference(buildImage, job, r.Scheme); err != nil {
		slog.Error("Failed to set controller reference", "error", err)
		return nil, err
	}

	// Get existing job
	existingJob := &batchv1.Job{}
	err = r.Get(ctx, client.ObjectKey{Namespace: job.Namespace, Name: job.Name}, existingJob)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if err := r.Create(ctx, job); err != nil {
			return nil, err
		}
		return job, nil

	}

	slog.Info("Found existing build job", "job", existingJob.Name, "namespace", existingJob.Namespace)
	return existingJob, nil
}

func (r *BuildImageController) updateBuildImageStatus(ctx context.Context, buildImage *buildimagev1alpha1.BuildImage, job *batchv1.Job) error {
	oldStatus := buildImage.Status

	// Determine status based on job condition
	newStatus := buildimagev1alpha1.BuildImageStatus{
		JobName: job.Name,
	}

	// Check job conditions for final status
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == "True" {
				newStatus.Status = "Succeeded"
				newStatus.Reason = "JobCompleted"
				newStatus.Contitions = append(newStatus.Contitions, metav1.Condition{
					Type:               "Complete",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "JobSucceeded",
					Message:            condition.Message,
				})
			}
		case batchv1.JobFailed:
			if condition.Status == "True" {
				newStatus.Status = "Failed"
				newStatus.Reason = condition.Reason
				newStatus.Contitions = append(newStatus.Contitions, metav1.Condition{
					Type:               "Failed",
					Status:             metav1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             condition.Reason,
					Message:            condition.Message,
				})
			}
		}
	}

	// If job is still running (Active > 0)
	if newStatus.Status == "" {
		if job.Status.Active > 0 {
			newStatus.Status = "Building"
			newStatus.Reason = "JobRunning"
		} else if job.Status.Failed > 0 {
			newStatus.Status = "Failed"
			newStatus.Reason = "JobFailed"
		} else if job.Status.Succeeded > 0 {
			newStatus.Status = "Succeeded"
			newStatus.Reason = "JobSucceeded"
		} else {
			newStatus.Status = "Pending"
			newStatus.Reason = "JobPending"
		}
	}

	// Check if status changed
	if oldStatus.Status == newStatus.Status && oldStatus.Reason == newStatus.Reason {
		return nil
	}

	// Update status
	buildImage.Status = newStatus
	if buildImage.Labels == nil {
		buildImage.Labels = make(map[string]string)
	}
	ifFinish := "false"
	if newStatus.Status == "Succeeded" || newStatus.Status == "Failed" {
		ifFinish = "true"
	}
	buildImage.Labels["w7.cc/build-finish"] = ifFinish
	buildImage.Labels["w7.cc/build-status"] = newStatus.Status
	if err := r.Update(ctx, buildImage); err != nil {
		slog.Error("Failed to update BuildImage status", "error", err)
		return err
	}

	slog.Info("Updated BuildImage status",
		"name", buildImage.Name,
		"namespace", buildImage.Namespace,
		"status", newStatus.Status,
		"reason", newStatus.Reason,
		"jobName", job.Name,
	)

	return nil
}

func (r *BuildImageController) handleDeletion(ctx context.Context, buildImage *buildimagev1alpha1.BuildImage) (ctrl.Result, error) {
	slog.Info("Handling BuildImage deletion", "name", buildImage.Name, "namespace", buildImage.Namespace)

	// Remove finalizer
	controllerutil.RemoveFinalizer(buildImage, BuildImageFinalizerName)
	if err := r.Update(ctx, buildImage); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}
