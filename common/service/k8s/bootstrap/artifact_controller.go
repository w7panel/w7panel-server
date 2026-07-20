package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ArtifactReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	installer artifactInstaller
	slots     *leaseSlots
}

func setupArtifactController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	installer, err := newZPKArtifactInstaller(sdk)
	if err != nil {
		return err
	}
	reconciler := &ArtifactReconciler{
		Client: mgr.GetClient(), Scheme: mgr.GetScheme(), installer: installer,
		slots: newLeaseSlotsWithReader(mgr.GetAPIReader(), mgr.GetClient(), sdk.GetNamespace()),
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.ArtifactInstallation{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 5}).
		Complete(reconciler)
}

func (r *ArtifactReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	installation := &bootstrapv1.ArtifactInstallation{}
	if err := r.Get(ctx, req.NamespacedName, installation); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !installation.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, installation)
	}
	if !controllerutil.ContainsFinalizer(installation, bootstrapv1.ArtifactFinalizer) {
		base := installation.DeepCopy()
		controllerutil.AddFinalizer(installation, bootstrapv1.ArtifactFinalizer)
		return ctrl.Result{}, r.Patch(ctx, installation, client.MergeFrom(base))
	}
	profile := &bootstrapv1.BootstrapProfile{}
	if err := r.Get(ctx, client.ObjectKey{Name: installation.Spec.ProfileRef.Name}, profile); err != nil {
		if apierrors.IsNotFound(err) {
			return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "所属 BootstrapProfile 不存在", 30*time.Second)
		}
		return ctrl.Result{}, err
	}
	if string(profile.UID) != installation.Spec.ProfileRef.UID {
		return ctrl.Result{}, r.updateStatus(ctx, installation, bootstrapv1.BootstrapPhaseFailed, "BootstrapProfile UID 不匹配，拒绝接管", nil, true)
	}
	if err := validateProfile(profile); err != nil {
		return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "等待 BootstrapProfile 校验通过: "+err.Error(), 30*time.Second)
	}
	if installation.Spec.ProfileRevision != profile.Spec.Revision {
		return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "等待 Profile Controller 同步当前 revision", 5*time.Second)
	}
	declared := false
	for _, artifact := range profile.Spec.Artifacts {
		if artifact.Name != installation.Spec.Artifact.Name {
			continue
		}
		declared = true
		if !reflect.DeepEqual(effectiveArtifact(profile, artifact), installation.Spec) {
			return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "等待 Profile Controller 修正被修改的 ArtifactInstallation", 5*time.Second)
		}
		break
	}
	if !declared {
		return ctrl.Result{}, r.updateStatus(ctx, installation, bootstrapv1.BootstrapPhaseFailed, "Profile 中不存在该应用声明，拒绝执行", nil, true)
	}
	settings := profileSettings(profile)

	if installation.Status.ObservedProfileRevision != "" && installation.Status.ObservedProfileRevision != installation.Spec.ProfileRevision {
		if installation.Status.OperationID != "" {
			_ = r.slots.release(ctx, installation.Status.OperationID)
		}
		if err := r.resetForRevision(ctx, installation); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	installed, err := r.installer.Lookup(ctx, installation)
	if err != nil {
		return ctrl.Result{}, err
	}
	if installed != nil && installed.State == installedArtifactDeleting {
		if installation.Status.OperationID != "" && r.slots != nil {
			_ = r.slots.release(ctx, installation.Status.OperationID)
		}
		return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "等待对应应用删除完成", 5*time.Second)
	}
	if installed != nil && normalizeIdentifie(installed.Identifie) != normalizeIdentifie(installation.Spec.Artifact.Identifie) {
		if installation.Status.OperationID != "" && r.slots != nil {
			_ = r.slots.release(ctx, installation.Status.OperationID)
		}
		return ctrl.Result{}, r.updateStatus(ctx, installation, bootstrapv1.BootstrapPhaseFailed,
			fmt.Sprintf("已安装应用 %s/%s identifie=%q 与 Profile 的 %q 冲突", installed.Namespace, installed.Name, installed.Identifie, installation.Spec.Artifact.Identifie), nil, true)
	}
	if installed != nil && installed.State == installedArtifactReady {
		if installation.Status.OperationID != "" && r.slots != nil {
			_ = r.slots.release(ctx, installation.Status.OperationID)
		}
		return ctrl.Result{}, r.updateStatus(ctx, installation, bootstrapv1.BootstrapPhaseReady, "AppGroup 已安装完成", installed, true)
	}
	if installed != nil && installed.State == installedArtifactFailed {
		if installation.Status.OperationID != "" && r.slots != nil {
			_ = r.slots.release(ctx, installation.Status.OperationID)
		}
		if installation.Status.Phase == bootstrapv1.BootstrapPhaseFailed && installation.Status.RetryCount >= settings.MaxRetries {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return r.retry(ctx, installation, settings, errors.New("AppGroup 安装失败"))
	}
	if installed != nil && installed.State == installedArtifactInstalling {
		if installation.Status.Phase == bootstrapv1.BootstrapPhaseFailed && installation.Status.RetryCount >= settings.MaxRetries {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if installation.Status.Phase == bootstrapv1.BootstrapPhaseInstalling && installation.Status.OperationID != "" {
			return r.waitForAppGroup(ctx, installation, settings)
		}
		return r.waitForExistingAppGroup(ctx, installation, settings)
	}
	if installation.Status.ObservedProfileRevision == installation.Spec.ProfileRevision && terminalPhase(installation.Status.Phase) {
		return ctrl.Result{}, nil
	}
	operationInProgress := installation.Status.Phase == bootstrapv1.BootstrapPhaseInstalling && installation.Status.OperationID != ""
	if operationInProgress {
		return r.waitForAppGroup(ctx, installation, settings)
	}

	ready, message, err := r.dependenciesReady(ctx, installation)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ready {
		return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, message, 10*time.Second)
	}

	operation := operationID(installation)
	acquired, err := r.slots.acquire(ctx, profile.Name, operation, settings.MaxConcurrent, settings.TimeoutPerArtifact)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !acquired {
		return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhasePending, "等待可用安装并发槽", 5*time.Second)
	}

	phase := bootstrapv1.BootstrapPhaseInstalling
	now := metav1.Now()
	installation.Status.ObservedProfileRevision = installation.Spec.ProfileRevision
	installation.Status.Phase = phase
	installation.Status.OperationID = operation
	installation.Status.StartedAt = &now
	installation.Status.CompletedAt = nil
	installation.Status.Message = "已提交 ZPK 安装操作"
	if err := r.Status().Update(ctx, installation); err != nil {
		// A conflict usually means another replica claimed this same logical
		// operation. It must keep the shared slot; releasing it here would steal
		// that replica's concurrency permit.
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
			return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhasePending, "已检测到对应应用，正在重新确认", time.Second)
		}
		if errors.Is(err, errArtifactDeleting) {
			return r.requeueWithStatus(ctx, installation, bootstrapv1.BootstrapPhaseBlocked, "等待对应应用删除完成", 5*time.Second)
		}
		return r.retry(ctx, installation, settings, err)
	}
	slog.Info("Bootstrap 应用操作已提交", "profile", profile.Name, "application", installation.Spec.Artifact.Name, "operationID", operation, "phase", phase)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ArtifactReconciler) reconcileDeletion(ctx context.Context, installation *bootstrapv1.ArtifactInstallation) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(installation, bootstrapv1.ArtifactFinalizer) {
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
	controllerutil.RemoveFinalizer(installation, bootstrapv1.ArtifactFinalizer)
	return ctrl.Result{}, r.Patch(ctx, installation, client.MergeFrom(base))
}

func (r *ArtifactReconciler) dependenciesReady(ctx context.Context, installation *bootstrapv1.ArtifactInstallation) (bool, string, error) {
	for _, dependency := range installation.Spec.DependsOn {
		item := &bootstrapv1.ArtifactInstallation{}
		name := artifactInstallationName(installation.Spec.ProfileRef.Name, dependency)
		if err := r.Get(ctx, client.ObjectKey{Name: name}, item); err != nil {
			if apierrors.IsNotFound(err) {
				return false, fmt.Sprintf("等待依赖 %q 展开", dependency), nil
			}
			return false, "", err
		}
		if item.Spec.ProfileRevision != installation.Spec.ProfileRevision {
			return false, fmt.Sprintf("等待依赖 %q 同步当前 revision", dependency), nil
		}
		switch item.Status.Phase {
		case bootstrapv1.BootstrapPhaseFailed:
			if item.Spec.FailurePolicy == bootstrapv1.FailurePolicyStop {
				return false, fmt.Sprintf("依赖 %q 未成功且策略要求停止: %s", dependency, item.Status.Phase), nil
			}
			// Continue 策略只记录失败，不阻止依赖任务。
			continue
		}
		if item.Status.Phase != bootstrapv1.BootstrapPhaseReady {
			return false, fmt.Sprintf("等待依赖 %q Ready", dependency), nil
		}
	}
	return true, "", nil
}

func (r *ArtifactReconciler) waitForAppGroup(ctx context.Context, installation *bootstrapv1.ArtifactInstallation, settings effectiveProfile) (ctrl.Result, error) {
	if installation.Status.StartedAt != nil && time.Since(installation.Status.StartedAt.Time) > settings.TimeoutPerArtifact {
		_ = r.slots.release(ctx, installation.Status.OperationID)
		return r.retry(ctx, installation, settings, fmt.Errorf("等待 AppGroup 就绪超过 %s", settings.TimeoutPerArtifact))
	}
	_, err := r.slots.acquire(ctx, installation.Spec.ProfileRef.Name, installation.Status.OperationID, settings.MaxConcurrent, settings.TimeoutPerArtifact)
	return ctrl.Result{RequeueAfter: 5 * time.Second}, err
}

func (r *ArtifactReconciler) waitForExistingAppGroup(ctx context.Context, installation *bootstrapv1.ArtifactInstallation, settings effectiveProfile) (ctrl.Result, error) {
	if installation.Status.StartedAt != nil && time.Since(installation.Status.StartedAt.Time) > settings.TimeoutPerArtifact {
		return r.retry(ctx, installation, settings, fmt.Errorf("等待已有 AppGroup 就绪超过 %s", settings.TimeoutPerArtifact))
	}
	base := installation.DeepCopy()
	installation.Status.ObservedProfileRevision = installation.Spec.ProfileRevision
	installation.Status.Phase = bootstrapv1.BootstrapPhaseInstalling
	installation.Status.Message = "等待已有 AppGroup 安装完成"
	if installation.Status.StartedAt == nil {
		now := metav1.Now()
		installation.Status.StartedAt = &now
	}
	if err := r.Status().Patch(ctx, installation, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ArtifactReconciler) retry(ctx context.Context, installation *bootstrapv1.ArtifactInstallation, settings effectiveProfile, cause error) (ctrl.Result, error) {
	base := installation.DeepCopy()
	if installation.Status.RetryCount < settings.MaxRetries {
		installation.Status.RetryCount++
	}
	installation.Status.Message = cause.Error()
	installation.Status.StartedAt = nil
	if installation.Status.RetryCount >= settings.MaxRetries {
		installation.Status.Phase = bootstrapv1.BootstrapPhaseFailed
		installation.Status.ObservedProfileRevision = installation.Spec.ProfileRevision
		now := metav1.Now()
		installation.Status.CompletedAt = &now
	} else {
		installation.Status.Phase = bootstrapv1.BootstrapPhasePending
	}
	if err := r.Status().Patch(ctx, installation, client.MergeFrom(base)); err != nil {
		return ctrl.Result{}, err
	}
	if installation.Status.Phase == bootstrapv1.BootstrapPhaseFailed {
		slog.Error("Bootstrap 应用执行失败", "profile", installation.Spec.ProfileRef.Name, "application", installation.Spec.Artifact.Name, "error", cause)
		// AppGroup may be repaired manually after reaching the retry limit. Keep a
		// low-frequency observation so the Bootstrap status can recover to Ready.
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	delay := time.Duration(1<<min(installation.Status.RetryCount, 6)) * time.Second
	return ctrl.Result{RequeueAfter: delay}, nil
}

func (r *ArtifactReconciler) resetForRevision(ctx context.Context, installation *bootstrapv1.ArtifactInstallation) error {
	base := installation.DeepCopy()
	appGroup := installation.Status.AppGroup
	installation.Status = bootstrapv1.ArtifactInstallationStatus{Phase: bootstrapv1.BootstrapPhasePending, AppGroup: appGroup}
	return r.Status().Patch(ctx, installation, client.MergeFrom(base))
}

func (r *ArtifactReconciler) updateStatus(ctx context.Context, installation *bootstrapv1.ArtifactInstallation, phase bootstrapv1.BootstrapPhase, message string, installed *installedArtifact, complete bool) error {
	base := installation.DeepCopy()
	installation.Status.Phase = phase
	installation.Status.Message = message
	installation.Status.ObservedProfileRevision = installation.Spec.ProfileRevision
	if installed != nil {
		installation.Status.InstalledVersion = installed.Version
		installation.Status.AppGroup = bootstrapv1.ArtifactAppGroupStatus{Name: installed.Name, Namespace: installed.Namespace}
	}
	if complete {
		now := metav1.Now()
		installation.Status.CompletedAt = &now
	}
	if reflect.DeepEqual(base.Status, installation.Status) {
		return nil
	}
	return r.Status().Patch(ctx, installation, client.MergeFrom(base))
}

func (r *ArtifactReconciler) requeueWithStatus(ctx context.Context, installation *bootstrapv1.ArtifactInstallation, phase bootstrapv1.BootstrapPhase, message string, after time.Duration) (ctrl.Result, error) {
	if err := r.updateStatus(ctx, installation, phase, message, nil, false); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: after}, nil
}
