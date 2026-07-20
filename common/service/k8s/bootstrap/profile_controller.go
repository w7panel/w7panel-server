package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type ProfileReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func setupProfileController(mgr ctrl.Manager) error {
	reconciler := &ProfileReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	return ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.BootstrapProfile{}).
		Owns(&bootstrapv1.ArtifactInstallation{}).
		Complete(reconciler)
}

func (r *ProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	profile := &bootstrapv1.BootstrapProfile{}
	if err := r.Get(ctx, req.NamespacedName, profile); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !profile.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	if err := validateProfile(profile); err != nil {
		slog.Warn("BootstrapProfile 校验失败", "profile", profile.Name, "error", err)
		return ctrl.Result{}, r.updateInvalidProfileStatus(ctx, profile, err)
	}

	installations := &bootstrapv1.ArtifactInstallationList{}
	if err := r.List(ctx, installations, client.MatchingLabels{bootstrapv1.LabelProfile: profile.Name}); err != nil {
		return ctrl.Result{}, fmt.Errorf("查询 ArtifactInstallation: %w", err)
	}
	mutations := 0
	desiredNames := make(map[string]struct{}, len(profile.Spec.Artifacts))
	for _, artifact := range profile.Spec.Artifacts {
		desiredNames[artifact.Name] = struct{}{}
	}
	for _, artifact := range profile.Spec.Artifacts {
		changed, err := r.syncInstallation(ctx, profile, artifact)
		if err != nil {
			return ctrl.Result{}, err
		}
		if changed {
			mutations++
		}
	}

	for i := range installations.Items {
		installation := &installations.Items[i]
		if _, desired := desiredNames[installation.Spec.Artifact.Name]; desired {
			continue
		}
		if err := r.Delete(ctx, installation); err != nil {
			return ctrl.Result{}, err
		}
		mutations++
	}

	if mutations > 0 {
		return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, r.updateProfileStatus(ctx, profile)
	}
	return ctrl.Result{}, r.updateProfileStatus(ctx, profile)
}

func (r *ProfileReconciler) syncInstallation(ctx context.Context, profile *bootstrapv1.BootstrapProfile, artifact bootstrapv1.BootstrapArtifact) (bool, error) {
	desiredSpec := effectiveArtifact(profile, artifact)
	installation := &bootstrapv1.ArtifactInstallation{ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name)}}
	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, installation, func() error {
		installation.Spec = desiredSpec
		if installation.Labels == nil {
			installation.Labels = map[string]string{}
		}
		installation.Labels[bootstrapv1.LabelProfile] = profile.Name
		installation.Labels[bootstrapv1.LabelArtifact] = artifact.Name
		controllerutil.AddFinalizer(installation, bootstrapv1.ArtifactFinalizer)
		return controllerutil.SetControllerReference(profile, installation, r.Scheme)
	})
	if err != nil {
		return false, fmt.Errorf("同步 ArtifactInstallation %q: %w", installation.Name, err)
	}
	return result != controllerutil.OperationResultNone, nil
}

func (r *ProfileReconciler) updateInvalidProfileStatus(ctx context.Context, profile *bootstrapv1.BootstrapProfile, validationErr error) error {
	base := profile.DeepCopy()
	profile.Status.Phase = bootstrapv1.ProfilePhaseInvalid
	profile.Status.ObservedRevision = profile.Spec.Revision
	profile.Status.Expansion = bootstrapv1.BootstrapExpansionStatus{Total: int32(len(profile.Spec.Artifacts))}
	setCondition(&profile.Status.Conditions, metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionFalse,
		ObservedGeneration: profile.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ValidationFailed",
		Message:            validationErr.Error(),
	})
	if reflect.DeepEqual(base.Status, profile.Status) {
		return nil
	}
	return r.Status().Patch(ctx, profile, client.MergeFrom(base))
}

func (r *ProfileReconciler) updateProfileStatus(ctx context.Context, profile *bootstrapv1.BootstrapProfile) error {
	installations := &bootstrapv1.ArtifactInstallationList{}
	if err := r.List(ctx, installations, client.MatchingLabels{bootstrapv1.LabelProfile: profile.Name}); err != nil {
		return err
	}
	base := profile.DeepCopy()
	desired := make(map[string]struct{}, len(profile.Spec.Artifacts))
	for _, artifact := range profile.Spec.Artifacts {
		desired[artifact.Name] = struct{}{}
	}
	summary := bootstrapv1.BootstrapSummary{Total: int32(len(profile.Spec.Artifacts))}
	processed := int32(0)
	for _, installation := range installations.Items {
		if _, ok := desired[installation.Spec.Artifact.Name]; !ok {
			continue
		}
		if installation.Spec.ProfileRevision == profile.Spec.Revision {
			processed++
		}
		// Spec 先由 Profile Controller 同步，Status 随后才由 Artifact
		// Controller 开始新一轮协调。旧 revision 的终态不能计入当前轮汇总，
		// 否则最后一批 Spec 更新后 Profile 会短暂地被错误标记为 Ready。
		if installation.Spec.ProfileRevision != profile.Spec.Revision ||
			installation.Status.ObservedProfileRevision != profile.Spec.Revision {
			summary.Progressing++
			continue
		}
		switch installation.Status.Phase {
		case bootstrapv1.BootstrapPhaseReady:
			summary.Ready++
		case bootstrapv1.BootstrapPhaseFailed:
			summary.Failed++
		case bootstrapv1.BootstrapPhaseBlocked:
			summary.Blocked++
		default:
			summary.Progressing++
		}
	}
	profile.Status.ObservedRevision = profile.Spec.Revision
	profile.Status.Expansion = bootstrapv1.BootstrapExpansionStatus{
		Total: int32(len(profile.Spec.Artifacts)), Processed: processed, Complete: processed == int32(len(profile.Spec.Artifacts)),
	}
	profile.Status.Summary = summary
	switch {
	case !profile.Status.Expansion.Complete:
		profile.Status.Phase = bootstrapv1.ProfilePhaseProgressing
	case summary.Failed > 0:
		profile.Status.Phase = bootstrapv1.ProfilePhaseDegraded
	case summary.Progressing > 0 || summary.Blocked > 0:
		profile.Status.Phase = bootstrapv1.ProfilePhaseProgressing
	default:
		profile.Status.Phase = bootstrapv1.ProfilePhaseReady
	}
	setCondition(&profile.Status.Conditions, metav1.Condition{
		Type:               "Valid",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: profile.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             "ValidationSucceeded",
		Message:            "BootstrapProfile 校验通过",
	})
	if reflect.DeepEqual(base.Status, profile.Status) {
		return nil
	}
	if err := r.Status().Patch(ctx, profile, client.MergeFrom(base)); err != nil && !apierrors.IsConflict(err) {
		return fmt.Errorf("更新 BootstrapProfile %q 状态: %w", profile.Name, err)
	}
	return nil
}
