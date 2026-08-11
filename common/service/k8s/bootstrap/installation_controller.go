package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type InstallationReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	installer artifactInstaller
	slots     *leaseSlots
}

const (
	updateCheckRetryInterval = time.Minute
	updateRetryCooldown      = 10 * time.Minute
)

func setupInstallationController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	installer, err := newZPKArtifactInstaller(sdk)
	if err != nil {
		return err
	}
	reconciler := &InstallationReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), installer: installer,
		slots: newLeaseSlotsWithReader(mgr.GetAPIReader(), mgr.GetClient(), sdk.GetNamespace()),
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&installationv1.BootstrapInstallation{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Complete(reconciler)
}

func (r *InstallationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	installation := &installationv1.BootstrapInstallation{}
	if err := r.Get(ctx, req.NamespacedName, installation); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !installation.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, installation)
	}
	if !controllerutil.ContainsFinalizer(installation, installationv1.InstallationFinalizer) {
		base := installation.DeepCopy()
		controllerutil.AddFinalizer(installation, installationv1.InstallationFinalizer)
		return ctrl.Result{}, r.Patch(ctx, installation, client.MergeFrom(base))
	}
	if err := validateInstallation(installation); err != nil {
		return ctrl.Result{}, r.updateStatus(ctx, installation, installationv1.BootstrapPhaseFailed, "BootstrapInstallation 配置无效: "+err.Error(), nil, true)
	}
	settings := installationSettings(installation)
	installed, err := r.installer.Lookup(ctx, installation)
	if err != nil {
		return ctrl.Result{}, err
	}
	if installed != nil {
		switch installed.State {
		case installedArtifactDeleting:
			r.releaseOperation(ctx, installation)
			return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhaseBlocked, "等待对应应用删除完成", 5*time.Second)
		case installedArtifactReady:
			r.releaseOperation(ctx, installation)
			return r.reconcileReadyAppGroup(ctx, installation, installed, settings)
		case installedArtifactFailed:
			return r.retryFailedAppGroup(ctx, installation, installed, settings)
		case installedArtifactInstalling:
			if installation.Status.Phase == installationv1.BootstrapPhaseInstalling && installation.Status.OperationID != "" {
				return r.waitForAppGroup(ctx, installation, settings)
			}
			return r.waitForExistingAppGroup(ctx, installation, installed, settings)
		}
	}
	if installation.Status.Phase == installationv1.BootstrapPhaseInstalling && installation.Status.OperationID != "" {
		return r.waitForAppGroup(ctx, installation, settings)
	}

	operation := operationID(installation)
	acquired, err := r.slots.acquire(ctx, bootstrapSlotScope, operation, settings.MaxConcurrent, settings.TimeoutPerArtifact)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !acquired {
		return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhasePending, "等待可用安装并发槽", 5*time.Second)
	}

	now := metav1.Now()
	installation.Status.Phase = installationv1.BootstrapPhaseInstalling
	installation.Status.OperationID = operation
	installation.Status.StartedAt = &now
	installation.Status.CompletedAt = nil
	installation.Status.Message = "已提交 ZPK 安装操作"
	if err := r.Status().Update(ctx, installation); err != nil {
		if !apierrors.IsConflict(err) {
			_ = r.slots.release(ctx, operation)
		}
		return ctrl.Result{}, err
	}
	installCtx, cancel := context.WithTimeout(ctx, settings.TimeoutPerArtifact)
	defer cancel()
	if err := r.installer.Install(installCtx, installation); err != nil {
		_ = r.slots.release(ctx, operation)
		if errors.Is(err, errArtifactAlreadyExists) {
			return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhasePending, "已检测到对应应用，正在重新确认", time.Second)
		}
		if errors.Is(err, errArtifactDeleting) {
			return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhaseBlocked, "等待对应应用删除完成", 5*time.Second)
		}
		return r.retry(ctx, installation, settings, err)
	}
	slog.Info("Bootstrap 应用操作已提交", "installation", installation.Name, "application", installation.Spec.Artifact.Name, "operationID", operation)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *InstallationReconciler) reconcileReadyAppGroup(ctx context.Context, installation *installationv1.BootstrapInstallation, installed *installedArtifact, settings effectiveSettings) (ctrl.Result, error) {
	if installation.Status.Phase == installationv1.BootstrapPhaseInstalling {
		message := "AppGroup 安装完成"
		if installation.Status.InstalledVersion != "" {
			message = "AppGroup 更新完成"
		}
		if err := r.updateReadyStatus(ctx, installation, installed, message); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	if !needsArtifactVersionLookup(installation.Spec.Artifact.Version, installed.Version) {
		if err := r.updateReadyStatus(ctx, installation, installed, fmt.Sprintf("当前已是指定版本 %q", installed.Version)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, settings.TimeoutPerArtifact)
	defer cancel()
	update, err := r.installer.ResolveUpdate(checkCtx, installation, installed.Version)
	if err != nil {
		slog.Warn("检查 Bootstrap 制品更新失败", "installation", installation.Name, "application", installation.Spec.Artifact.Name, "error", err)
		return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhaseReady,
			"检查 ZPK 制品更新失败: "+err.Error(), updateCheckRetryInterval)
	}

	operation := operationID(installation)
	acquired, err := r.slots.acquire(ctx, bootstrapSlotScope, operation, settings.MaxConcurrent, settings.TimeoutPerArtifact)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !acquired {
		return r.requeueWithStatus(ctx, installation, installationv1.BootstrapPhasePending, "等待可用更新并发槽", 5*time.Second)
	}

	now := metav1.Now()
	installation.Status.Phase = installationv1.BootstrapPhaseInstalling
	installation.Status.OperationID = operation
	installation.Status.InstalledVersion = installed.Version
	installation.Status.AppGroup = installationv1.ArtifactAppGroupStatus{Name: installed.Name, Namespace: installed.Namespace}
	installation.Status.StartedAt = &now
	installation.Status.CompletedAt = nil
	installation.Status.Message = fmt.Sprintf("正在将 AppGroup 从版本 %q 更新到 %q", installed.Version, update.Version)
	if err := r.Status().Update(ctx, installation); err != nil {
		if !apierrors.IsConflict(err) {
			_ = r.slots.release(ctx, operation)
		}
		return ctrl.Result{}, err
	}

	upgradeCtx, cancel := context.WithTimeout(ctx, settings.TimeoutPerArtifact)
	defer cancel()
	if err := r.installer.Upgrade(upgradeCtx, installation, update); err != nil {
		_ = r.slots.release(ctx, operation)
		return r.retry(ctx, installation, settings, fmt.Errorf("更新 AppGroup: %w", err))
	}
	slog.Info("Bootstrap 应用更新已提交", "installation", installation.Name, "application", installation.Spec.Artifact.Name,
		"currentVersion", installed.Version, "targetVersion", update.Version, "operationID", operation)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *InstallationReconciler) updateReadyStatus(ctx context.Context, installation *installationv1.BootstrapInstallation, installed *installedArtifact, message string) error {
	base := installation.DeepCopy()
	installation.Status.Phase = installationv1.BootstrapPhaseReady
	installation.Status.InstalledVersion = installed.Version
	installation.Status.AppGroup = installationv1.ArtifactAppGroupStatus{Name: installed.Name, Namespace: installed.Namespace}
	installation.Status.OperationID = ""
	installation.Status.RetryCount = 0
	installation.Status.Message = message
	installation.Status.StartedAt = nil
	if base.Status.Phase != installationv1.BootstrapPhaseReady || installation.Status.CompletedAt == nil || base.Status.InstalledVersion != installed.Version {
		now := metav1.Now()
		installation.Status.CompletedAt = &now
	}
	if reflect.DeepEqual(base.Status, installation.Status) {
		return nil
	}
	return r.Status().Patch(ctx, installation, client.MergeFrom(base))
}

func (r *InstallationReconciler) retryFailedAppGroup(ctx context.Context, installation *installationv1.BootstrapInstallation, installed *installedArtifact, settings effectiveSettings) (ctrl.Result, error) {
	r.releaseOperation(ctx, installation)
	cause := errors.New("AppGroup 安装失败")
	if installation.Status.RetryCount >= settings.MaxRetries {
		return ctrl.Result{}, nil
	}
	if !installed.Owned {
		return ctrl.Result{}, r.updateStatus(ctx, installation, installationv1.BootstrapPhaseFailed,
			"已有非 Bootstrap 管理的 AppGroup 安装失败，不能自动删除并重试", installed, true)
	}
	if err := r.installer.Uninstall(ctx, installation); err != nil {
		return ctrl.Result{}, fmt.Errorf("清理失败的 AppGroup 以便重试: %w", err)
	}
	return r.retry(ctx, installation, settings, cause)
}

func (r *InstallationReconciler) releaseOperation(ctx context.Context, installation *installationv1.BootstrapInstallation) {
	if installation.Status.OperationID != "" && r.slots != nil {
		_ = r.slots.release(ctx, installation.Status.OperationID)
	}
}

func (r *InstallationReconciler) reconcileDeletion(ctx context.Context, installation *installationv1.BootstrapInstallation) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(installation, installationv1.InstallationFinalizer) {
		return ctrl.Result{}, nil
	}
	if err := r.installer.Uninstall(ctx, installation); err != nil {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	if installation.Status.OperationID != "" && r.slots != nil {
		if err := r.slots.release(ctx, installation.Status.OperationID); err != nil {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("释放 Bootstrap 安装并发槽: %w", err)
		}
	}
	base := installation.DeepCopy()
	controllerutil.RemoveFinalizer(installation, installationv1.InstallationFinalizer)
	return ctrl.Result{}, r.Patch(ctx, installation, client.MergeFrom(base))
}

func (r *InstallationReconciler) waitForAppGroup(ctx context.Context, installation *installationv1.BootstrapInstallation, settings effectiveSettings) (ctrl.Result, error) {
	if installation.Status.StartedAt != nil && time.Since(installation.Status.StartedAt.Time) > settings.TimeoutPerArtifact {
		_ = r.slots.release(ctx, installation.Status.OperationID)
		if installation.Status.RetryCount < settings.MaxRetries {
			if err := r.installer.Uninstall(ctx, installation); err != nil {
				return ctrl.Result{}, fmt.Errorf("清理超时的 AppGroup 以便重试: %w", err)
			}
		}
		return r.retry(ctx, installation, settings, fmt.Errorf("等待 AppGroup 就绪超过 %s", settings.TimeoutPerArtifact))
	}
	_, err := r.slots.acquire(ctx, bootstrapSlotScope, installation.Status.OperationID, settings.MaxConcurrent, settings.TimeoutPerArtifact)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, err
}

func (r *InstallationReconciler) waitForExistingAppGroup(ctx context.Context, installation *installationv1.BootstrapInstallation, installed *installedArtifact, settings effectiveSettings) (ctrl.Result, error) {
	if installation.Status.StartedAt != nil && time.Since(installation.Status.StartedAt.Time) > settings.TimeoutPerArtifact {
		if installed != nil && installed.Owned && installation.Status.RetryCount < settings.MaxRetries {
			if err := r.installer.Uninstall(ctx, installation); err != nil {
				return ctrl.Result{}, fmt.Errorf("清理超时的已有 AppGroup 以便重试: %w", err)
			}
		}
		return r.retry(ctx, installation, settings, fmt.Errorf("等待已有 AppGroup 就绪超过 %s", settings.TimeoutPerArtifact))
	}
	base := installation.DeepCopy()
	installation.Status.Phase = installationv1.BootstrapPhaseInstalling
	installation.Status.Message = "等待已有 AppGroup 安装完成"
	if base.Status.Phase != installationv1.BootstrapPhaseInstalling || installation.Status.StartedAt == nil {
		now := metav1.Now()
		installation.Status.StartedAt = &now
		installation.Status.CompletedAt = nil
	}
	if err := r.Status().Patch(ctx, installation, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *InstallationReconciler) retry(ctx context.Context, installation *installationv1.BootstrapInstallation, settings effectiveSettings, cause error) (ctrl.Result, error) {
	base := installation.DeepCopy()
	installation.Status.Message = cause.Error()
	installation.Status.StartedAt = nil
	if installation.Status.RetryCount >= settings.MaxRetries {
		installation.Status.Phase = installationv1.BootstrapPhaseFailed
		now := metav1.Now()
		installation.Status.CompletedAt = &now
	} else {
		installation.Status.RetryCount++
		installation.Status.Phase = installationv1.BootstrapPhasePending
		installation.Status.CompletedAt = nil
	}
	if err := r.Status().Patch(ctx, installation, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}
	if installation.Status.Phase == installationv1.BootstrapPhaseFailed {
		slog.Error("Bootstrap 应用执行失败", "installation", installation.Name, "application", installation.Spec.Artifact.Name, "error", cause)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	delay := time.Duration(1<<min(installation.Status.RetryCount, 6)) * time.Second
	return ctrl.Result{RequeueAfter: delay}, nil
}

func (r *InstallationReconciler) updateStatus(ctx context.Context, installation *installationv1.BootstrapInstallation, phase installationv1.BootstrapPhase, message string, installed *installedArtifact, complete bool) error {
	base := installation.DeepCopy()
	installation.Status.Phase = phase
	installation.Status.Message = message
	if installed != nil {
		installation.Status.InstalledVersion = installed.Version
		installation.Status.AppGroup = installationv1.ArtifactAppGroupStatus{Name: installed.Name, Namespace: installed.Namespace}
	}
	if complete && (base.Status.Phase != phase || installation.Status.CompletedAt == nil) {
		now := metav1.Now()
		installation.Status.CompletedAt = &now
	}
	if reflect.DeepEqual(base.Status, installation.Status) {
		return nil
	}
	return r.Status().Patch(ctx, installation, client.MergeFrom(base))
}

func (r *InstallationReconciler) requeueWithStatus(ctx context.Context, installation *installationv1.BootstrapInstallation, phase installationv1.BootstrapPhase, message string, after time.Duration) (ctrl.Result, error) {
	if err := r.updateStatus(ctx, installation, phase, message, nil, false); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: after}, nil
}
