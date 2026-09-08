package installcontroller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic"
	api "github.com/w7panel/w7panel/k8s/pkg/apis/zpkinstall/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const heartbeatInterval = 30 * time.Second
const heartbeatTimeout = 120 * time.Second
const retryInterval = 30 * time.Second

const EnabledEnv = "ZPKINSTALL_CONTROLLER_ENABLED"

// Enabled reports whether the ZpkInstall controller should be registered.
// It is enabled by default so existing deployments retain their behavior.
func Enabled() bool {
	value, found := os.LookupEnv(EnabledEnv)
	if !found || strings.TrimSpace(value) == "" {
		return true
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("invalid ZpkInstall controller enabled value; defaulting to enabled", "env", EnabledEnv, "value", value)
		return true
	}
	return enabled
}

type Executor func(context.Context, *api.ZpkInstall) (logic.InstallResult, error)

type Controller struct {
	client.Client
	Reader  client.Reader // Uncached reads are essential for claims and heartbeats.
	Execute Executor
	Now     func() time.Time
}

func Setup(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	r := &Controller{Client: mgr.GetClient(), Reader: mgr.GetAPIReader(), Now: time.Now}
	r.Execute = r.localExecutor(sdk)
	return ctrl.NewControllerManagedBy(mgr).For(&api.ZpkInstall{}).Complete(r)
}

func (r *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	task := &api.ZpkInstall{}
	if err := r.Reader.Get(ctx, req.NamespacedName, task); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !task.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	switch task.Status.Phase {
	case "Succeeded", "Failed", "Unknown":
		return ctrl.Result{}, nil
	case "Running":
		last := task.Status.HeartbeatAt
		if last == nil {
			last = task.Status.StartedAt
		}
		if last == nil || r.Now().Sub(last.Time) > heartbeatTimeout {
			now := metav1.NewTime(r.Now())
			task.Status.Phase, task.Status.Reason = "Unknown", "ExecutionInterrupted"
			task.Status.Message = "Execution result is unknown; inspect AppGroup and Jobs before creating another task."
			task.Status.CompletedAt = &now
			return ctrl.Result{}, r.Status().Update(ctx, task)
		}
		return ctrl.Result{RequeueAfter: heartbeatInterval}, nil
	case "", "Pending":
	default:
		return ctrl.Result{}, fmt.Errorf("unsupported installation phase")
	}
	now := metav1.NewTime(r.Now())
	// Preserve retryCount across attempts. A retry creates a fresh durable claim,
	// execution ID and install ID, so a result from an earlier attempt cannot win.
	task.Status.Phase = "Running"
	task.Status.ExecutionID = string(task.UID) + "-" + helper.RandomString(16)
	task.Status.InstallID = helper.RandomString(5)
	task.Status.StartedAt, task.Status.HeartbeatAt, task.Status.CompletedAt = &now, &now, nil
	task.Status.Reason, task.Status.Message = "", ""
	// This durable claim MUST succeed before any external installation effect.
	if err := r.Status().Update(ctx, task); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	execCtx, cancel := context.WithCancel(ctx)
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-execCtx.Done():
				return
			case <-ticker.C:
				if err := r.updateOwned(execCtx, task, func(current *api.ZpkInstall) {
					now := metav1.NewTime(r.Now())
					current.Status.HeartbeatAt = &now
				}); err != nil {
					cancel()
					return
				}
			}
		}
	}()
	result, execErr := executeSafely(execCtx, r.Execute, task.DeepCopy())
	close(stop)
	<-stopped
	if execCtx.Err() != nil {
		cancel()
		return ctrl.Result{RequeueAfter: heartbeatInterval}, nil
	}
	cancel()
	if execErr != nil {
		slog.Error("ZpkInstall installation failed",
			"namespace", task.Namespace,
			"name", task.Name,
			"executionId", task.Status.ExecutionID,
			"installId", task.Status.InstallID,
			"releaseName", task.Spec.ReleaseName,
			"repoUrl", task.Spec.RepoURL,
			"error", execErr)
	}
	err := r.updateOwned(ctx, task, func(current *api.ZpkInstall) {
		now := metav1.NewTime(r.Now())
		current.Status.CompletedAt = &now
		if execErr != nil {
			if current.Status.RetryCount < current.Spec.MaxRetries {
				current.Status.RetryCount++
				current.Status.Phase, current.Status.Reason = "Pending", "RetryScheduled"
				current.Status.Message = fmt.Sprintf("Installation attempt failed; retry %d of %d is scheduled.", current.Status.RetryCount, current.Spec.MaxRetries)
				current.Status.ExecutionID, current.Status.HeartbeatAt, current.Status.CompletedAt = "", nil, nil
				return
			}
			current.Status.Phase, current.Status.Reason = "Failed", "InstallationFailed"
			var credentialErr *credentialError
			if errors.As(execErr, &credentialErr) {
				current.Status.Reason = "CredentialUnavailable"
			}
			var conflict *logic.ArtifactInstallConflictError
			if errors.As(execErr, &conflict) {
				current.Status.Reason = "ArtifactInstallConflict"
			}
			// Repository errors can embed URLs, credentials, scripts and response bodies.
			current.Status.Message = "Installation failed; inspect application resources and supplied configuration before creating a new task."
		} else {
			current.Status.Phase, current.Status.Reason = "Succeeded", "InstallationSubmitted"
			current.Status.Message = "Installation submitted; application readiness is tracked by AppGroup and Jobs."
			current.Status.ReleaseName, current.Status.Namespace, current.Status.InstallID = result.ReleaseName, result.Namespace, result.InstallID
		}
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	if execErr != nil && task.Status.RetryCount < task.Spec.MaxRetries {
		return ctrl.Result{RequeueAfter: retryInterval}, nil
	}
	return ctrl.Result{}, nil
}

func executeSafely(ctx context.Context, execute Executor, task *api.ZpkInstall) (result logic.InstallResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("installation panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return execute(ctx, task)
}

func (r *Controller) updateOwned(ctx context.Context, claimed *api.ZpkInstall, update func(*api.ZpkInstall)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current := &api.ZpkInstall{}
		if err := r.Reader.Get(ctx, client.ObjectKeyFromObject(claimed), current); err != nil {
			return err
		}
		if current.UID != claimed.UID || current.Status.Phase != "Running" || current.Status.ExecutionID != claimed.Status.ExecutionID || !current.DeletionTimestamp.IsZero() {
			return errors.New("installation claim is no longer active")
		}
		update(current)
		return r.Status().Update(ctx, current)
	})
}

type credentialError struct{}

func (*credentialError) Error() string { return "installation credential is unavailable" }

func (r *Controller) secretValue(ctx context.Context, namespace string, ref *corev1.SecretKeySelector) (string, error) {
	if ref == nil {
		return "", nil
	}
	if ref.Name == "" || ref.Key == "" {
		return "", &credentialError{}
	}
	secret := &corev1.Secret{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, secret); err != nil {
		return "", &credentialError{}
	}
	value := secret.Data[ref.Key]
	if len(value) == 0 {
		return "", &credentialError{}
	}
	return string(value), nil
}

func requestFor(task *api.ZpkInstall) (logic.InstallRequest, error) {
	var request logic.InstallRequest
	data, err := json.Marshal(task.Spec)
	if err != nil {
		return request, err
	}
	if err = json.Unmarshal(data, &request); err != nil {
		return request, err
	}
	if request.Namespace == "" {
		request.Namespace = task.Namespace
	}
	if request.Namespace == "" || request.RepoUrl == "" || request.ReleaseName == "" || request.InstallOptions == nil {
		return request, errors.New("invalid installation request")
	}
	return request, nil
}

func (r *Controller) localExecutor(sdk *k8s.Sdk) Executor {
	return func(ctx context.Context, task *api.ZpkInstall) (logic.InstallResult, error) {
		request, err := requestFor(task)
		if err != nil {
			return logic.InstallResult{}, err
		}
		request.ThirdpartyCDToken, err = r.secretValue(ctx, task.Namespace, task.Spec.ThirdpartyCDTokenRef)
		if err != nil {
			return logic.InstallResult{}, err
		}
		panelToken, err := r.secretValue(ctx, task.Namespace, task.Spec.PanelTokenRef)
		if err != nil {
			return logic.InstallResult{}, err
		}
		base, err := sdk.ToRESTConfig()
		if err != nil {
			return logic.InstallResult{}, err
		}
		config := rest.CopyConfig(base)
		// Use only the local Server identity, never the repository's optional token.
		if config.BearerTokenFile != "" {
			token, err := os.ReadFile(config.BearerTokenFile)
			if err != nil {
				return logic.InstallResult{}, &credentialError{}
			}
			config.BearerToken = string(token)
		}
		// The local Kubernetes credential authenticates this controller to the
		// API server; it is not a W7Panel user token. Only an explicitly supplied
		// panel token may be used for user labels and Helm panel-token settings.
		var identity *k8s.K8sToken
		if strings.TrimSpace(panelToken) != "" {
			identity = k8s.NewK8sToken(panelToken)
		}
		local, err := k8s.NewForRestConfig(config, request.Namespace)
		if err != nil {
			return logic.InstallResult{}, err
		}
		local.Ctx = ctx
		return logic.ExecuteInstall(request, logic.InstallExecution{SDK: local, Identity: identity, PanelToken: panelToken,
			IsChild: helper.IsChildAgent(), InstallID: task.Status.InstallID})
	}
}
