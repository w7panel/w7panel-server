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
	client := mgr.GetClient()
	r := &BuildImageController{
		Client: client,
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
		TaskID:    buildImage.Spec.TaskID,
		Namespace: buildImage.Spec.Namespace,
		Source: struct {
			DownloadURL    string "json:\"downloadUrl\""
			DockerfilePath string "json:\"dockerfilePath\""
		}{
			DownloadURL:    buildImage.Spec.Source.DownloadURL,
			DockerfilePath: buildImage.Spec.Source.DockerfilePath,
		},
		TargetImage: struct {
			Address string "json:\"address\""
			Auth    struct {
				Username string "json:\"username\""
				Password string "json:\"password\""
			} "json:\"auth\""
		}{
			Address: buildImage.Spec.TargetImage.Address,
			Auth: struct {
				Username string "json:\"username\""
				Password string "json:\"password\""
			}{
				Username: buildImage.Spec.TargetImage.Auth.Username,
				Password: buildImage.Spec.TargetImage.Auth.Password,
			},
		},
		NotifyURL: buildImage.Spec.NotifyURL,
	}

	// Get panel registry IP
	panelIp, err := getPanelRegistryIp()
	if err != nil {
		logger.Error(err, "Failed to get panel registry IP")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Create or update the build job
	job, err := r.createOrUpdateBuildJob(ctx, buildImage, &spec, panelIp)
	if err != nil {
		logger.Error(err, "Failed to create/update build job")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Check job status
	if err := r.handleJobStatus(ctx, buildImage, job); err != nil {
		logger.Error(err, "Failed to handle job status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update BuildImage status based on job status
	if err := r.updateBuildImageStatus(ctx, buildImage, job); err != nil {
		logger.Error(err, "Failed to update BuildImage status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

func (r *BuildImageController) createOrUpdateBuildJob(ctx context.Context, buildImage *buildimagev1alpha1.BuildImage, spec *BuildImageSpec, panelIp string) (*batchv1.Job, error) {
	job := toBuildJob(*spec)
	job.Labels["build-image-uid"] = string(buildImage.UID)

	// Set controller reference
	if err := ctrl.SetControllerReference(buildImage, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Get existing job
	existingJob := &batchv1.Job{}
	err := r.Get(ctx, client.ObjectKey{Namespace: job.Namespace, Name: job.Name}, existingJob)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("failed to get existing job: %w", err)
	}

	// Create or update job
	if client.IgnoreNotFound(err) == nil {
		// Job exists, update if needed
		existingJob.Labels["build-image-uid"] = string(buildImage.UID)
		existingJob.Spec = job.Spec
		if err := r.Update(ctx, existingJob); err != nil {
			return nil, fmt.Errorf("failed to update job: %w", err)
		}
		return existingJob, nil
	}

	// Create new job
	if err := r.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}
	slog.Info("Created build job", "job", job.Name, "namespace", job.Namespace)
	return job, nil
}

func (r *BuildImageController) handleJobStatus(ctx context.Context, buildImage *buildimagev1alpha1.BuildImage, job *batchv1.Job) error {
	// Job status handling logic can be expanded based on requirements
	slog.Info("Handling job status", "job", job.Name, "namespace", job.Namespace)
	return nil
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
	if err := r.Status().Update(ctx, buildImage); err != nil {
		return fmt.Errorf("failed to update BuildImage status: %w", err)
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
