package site

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	appgroupv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	sitev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/site/v1alpha1"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SetupSiteController sets up the Site controller with the manager.
func SetupSiteController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	if err := sitev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := appgroupv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}

	k8sClient := mgr.GetClient()
	r := &SiteController{
		Client: k8sClient,
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&sitev1alpha1.Site{}).
		Complete(r)
}

// SiteController reconciles Site objects.
type SiteController struct {
	client.Client
	Scheme *runtime.Scheme
	*k8s.Sdk
}

var (
	registerSiteZpkWithName       = console.RegisterSiteZpkWithName
	registerSiteZpkOpenIdWithName = console.RegisterSiteZpkOpenIdWithName
)

func registerSite(site *sitev1alpha1.Site, openID string) (*console.AppSecret, error) {
	if openID == "" {
		return registerSiteZpkWithName(site.Spec.Host, site.Spec.SiteIdentifier, site.Spec.SiteName)
	}
	return registerSiteZpkOpenIdWithName(site.Spec.Host, site.Spec.SiteIdentifier, openID, site.Spec.SiteName)
}

func (r *SiteController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("Site reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
		)
	}
	return result, err
}

func (r *SiteController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("Recovered from panic in Site Handle", "panic", rec)
		}
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling Site", "name", req.Name)

	site := &sitev1alpha1.Site{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Site")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if !site.DeletionTimestamp.IsZero() {
		logger.Info("Site is being deleted", "name", req.Name)
		return ctrl.Result{}, nil
	}

	if siteIdentifierChanged(site) {
		slog.Info("SiteIdentifier changed, resetting registration", "name", site.GetName(), "oldSiteIdentifier", site.Status.ObservedSiteIdentifier, "newSiteIdentifier", site.Spec.SiteIdentifier)
		resetRegistrationForSiteIdentifier(site)
		if err := r.Status().Update(ctx, site); err != nil {
			logger.Error(err, "Failed to reset site registration after SiteIdentifier change")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// ── State machine ──────────────────────────────────────────
	switch site.Status.Phase {
	case "", "Pending":
		return r.handlePending(ctx, site)
	case "AppIdReady":
		return r.handleAppIdReady(ctx, site)
	case "Completed", "Failed":
		logger.Info("Site reached terminal phase, skipping", "phase", site.Status.Phase)
		return ctrl.Result{}, nil
	default:
		logger.Info("Unknown phase, resetting to Pending", "phase", site.Status.Phase)
		site.Status.Phase = "Pending"
		if err := r.Status().Update(ctx, site); err != nil {
			logger.Error(err, "Failed to reset site phase")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}
}

// handlePending registers the site via ZPK and transitions to AppIdReady or Failed.
func (r *SiteController) handlePending(ctx context.Context, site *sitev1alpha1.Site) (ctrl.Result, error) {
	// Persist the identifier being attempted so failed registrations retain a
	// baseline for detecting a later SiteIdentifier change.
	if site.Status.ObservedSiteIdentifier == "" {
		site.Status.ObservedSiteIdentifier = site.Spec.SiteIdentifier
	}

	// Child agent: sync to root panel instead of processing locally
	if helper.IsChildAgent() {
		slog.Info("Child agent mode, syncing site to root panel", "name", site.GetName())
		if err := k3k.SyncSiteHttp(site); err != nil {
			slog.Error("Failed to sync site to root panel", "name", site.GetName(), "error", err)
			r.setPhase(site, "Failed", "SyncFailed", metav1.ConditionFalse, fmt.Sprintf("sync site error: %s", err.Error()))
			if err := r.Status().Update(ctx, site); err != nil {
				slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		slog.Info("Site synced to root panel successfully", "name", site.GetName())
		r.setPhase(site, "Completed", "Synced", metav1.ConditionTrue, "site synced to root panel")
		if err := r.Status().Update(ctx, site); err != nil {
			slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}
	slog.Info("Registering site via ZPK", "name", site.GetName())
	var secret *console.AppSecret
	var err error
	if site.Spec.UserName == "" {
		secret, err = registerSite(site, "")
	} else {
		w7config, configErr := config.NewW7ConfigRepository(r.Sdk).Get(site.Spec.UserName)
		openID, openIDErr := siteUserOpenID(w7config, configErr)
		if openIDErr != nil {
			slog.Error("Failed to get OpenID for site registration", "name", site.GetName(), "userName", site.Spec.UserName, "error", openIDErr)
			r.setPhase(site, "Failed", "OpenIDUnavailable", metav1.ConditionFalse, fmt.Sprintf("get OpenID for user %q: %s", site.Spec.UserName, openIDErr.Error()))
			if statusErr := r.Status().Update(ctx, site); statusErr != nil {
				slog.Error("Failed to update site status", "name", site.GetName(), "error", statusErr)
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		secret, err = registerSite(site, openID)
	}
	if err != nil {
		site.Status.RegisterRetryCount++
		if site.Status.RegisterRetryCount >= 3 {
			slog.Error("Registration failed after 3 retries, giving up", "name", site.GetName(), "error", err)
			r.setPhase(site, "Failed", "RegistrationFailed", metav1.ConditionFalse, fmt.Sprintf("register site zpk error after 3 retries: %s", err.Error()))
			if err := r.Status().Update(ctx, site); err != nil {
				slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		slog.Warn("Registration failed, will retry", "name", site.GetName(), "error", err, "retry", site.Status.RegisterRetryCount)
		if err := r.Status().Update(ctx, site); err != nil {
			slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	now := metav1.Now()
	site.Status.AppId = secret.AppId
	site.Status.AppSecret = secret.AppSecret
	site.Status.LastRegisteredAt = &now
	r.setPhase(site, "AppIdReady", "AppIdReady", metav1.ConditionTrue, "AppId/AppSecret obtained from ZPK registration")

	slog.Info("Site registered successfully, phase -> AppIdReady", "name", site.GetName(), "appId", secret.AppId)
	if err := r.Status().Update(ctx, site); err != nil {
		slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return ctrl.Result{RequeueAfter: time.Second * 5}, nil
}

func siteUserOpenID(w7config *config.W7Config, configErr error) (string, error) {
	if configErr != nil {
		return "", fmt.Errorf("get user cloud config: %w", configErr)
	}
	if w7config == nil {
		return "", fmt.Errorf("user cloud config is empty")
	}
	if w7config.UserInfo == nil {
		return "", fmt.Errorf("user info is empty")
	}
	if w7config.UserInfo.OpenId == "" {
		return "", fmt.Errorf("user OpenID is empty")
	}
	return w7config.UserInfo.OpenId, nil
}

// siteIdentifierChanged reports whether the current registration state belongs
// to a different SiteIdentifier. Legacy terminal objects without an observed
// identifier are re-registered once to establish that baseline.
func siteIdentifierChanged(site *sitev1alpha1.Site) bool {
	if site.Status.ObservedSiteIdentifier != "" {
		return site.Status.ObservedSiteIdentifier != site.Spec.SiteIdentifier
	}
	return site.Status.Phase != "" || site.Status.AppId != "" || site.Status.AppSecret != ""
}

// resetRegistrationForSiteIdentifier discards credentials and retry state that
// belong to the previous SiteIdentifier, then starts a fresh registration.
func resetRegistrationForSiteIdentifier(site *sitev1alpha1.Site) {
	site.Status.AppId = ""
	site.Status.AppSecret = ""
	site.Status.LastRegisteredAt = nil
	site.Status.RegisterRetryCount = 0
	site.Status.PatchRetryCount = 0
	site.Status.ObservedSiteIdentifier = site.Spec.SiteIdentifier
	setSitePhase(site, "Pending", "SiteIdentifierChanged", metav1.ConditionUnknown, "siteIdentifier changed; registration will be retried")
}

// handleAppIdReady patches the target resource and transitions to Completed or Failed.
func (r *SiteController) handleAppIdReady(ctx context.Context, site *sitev1alpha1.Site) (ctrl.Result, error) {
	// If no target specified, skip directly to Completed
	if site.Spec.Target == nil {
		slog.Info("No target specified, skipping patch", "name", site.GetName())
		r.setPhase(site, "Completed", "NoTarget", metav1.ConditionTrue, "no target resource to patch")
		if err := r.Status().Update(ctx, site); err != nil {
			slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	slog.Info("Patching target resource with AppId/AppSecret", "name", site.GetName())

	err := r.patchTargetResource(site.Spec.Target, &console.AppSecret{AppId: site.Status.AppId, AppSecret: site.Status.AppSecret})
	if err != nil {
		if errors.IsNotFound(err) {
			site.Status.PatchRetryCount++
			if site.Status.PatchRetryCount >= 3 {
				slog.Warn("Target resource not found after 3 retries, giving up", "name", site.GetName(), "kind", site.Spec.Target.Kind)
				r.setPhase(site, "Failed", "TargetNotFound", metav1.ConditionFalse, "target resource not found after 3 retries")
				if err := r.Status().Update(ctx, site); err != nil {
					slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
					return ctrl.Result{RequeueAfter: time.Minute}, nil
				}
				return ctrl.Result{}, nil
			}
			slog.Info("Target resource not found, will retry", "name", site.GetName(), "kind", site.Spec.Target.Kind, "retry", site.Status.PatchRetryCount)
			if err := r.Status().Update(ctx, site); err != nil {
				slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		slog.Error("Failed to patch target resource", "name", site.GetName(), "error", err)
		r.setPhase(site, "Failed", "PatchFailed", metav1.ConditionFalse, fmt.Sprintf("patch target error: %s", err.Error()))
		if err := r.Status().Update(ctx, site); err != nil {
			slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	slog.Info("Target resource patched successfully, phase -> Completed", "name", site.GetName())
	r.setPhase(site, "Completed", "Patched", metav1.ConditionTrue, "target resource patched with AppId/AppSecret")
	if err := r.Status().Update(ctx, site); err != nil {
		slog.Error("Failed to update site status", "name", site.GetName(), "error", err)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return ctrl.Result{}, nil
}

// patchTargetResource injects APP_ID and APP_SECRET into the target resource.
// For Deployment/DaemonSet/StatefulSet: patches env vars via strategic merge.
// For AppGroup: updates spec.appCredentials on the AppGroup CR.
// For Secret/ConfigMap: creates or updates the resource with the credentials.
func (r *SiteController) patchTargetResource(target *sitev1alpha1.TargetRef, appSecret *console.AppSecret) error {
	switch strings.ToLower(target.Kind) {
	case "deployment", "daemonset", "statefulset":
		return r.patchWorkload(target.Namespace, target.Name, target.Kind, target.ContainerName, appSecret)
	case "appgroup":
		return r.patchAppGroup(target, appSecret)
	case "secret":
		return r.createOrUpdateSecret(target, appSecret)
	case "configmap":
		return r.createOrUpdateConfigMap(target, appSecret)
	default:
		return fmt.Errorf("unsupported target kind %q (supported: Deployment, DaemonSet, StatefulSet, AppGroup, Secret, ConfigMap)", target.Kind)
	}
}

// patchWorkload injects APP_ID and APP_SECRET env vars into a workload
// (Deployment, DaemonSet, StatefulSet) using a strategic merge patch.
func (r *SiteController) patchWorkload(namespace, name, kind, containerName string, appSecret *console.AppSecret) error {
	patchData, err := json.Marshal(map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []map[string]any{
						{
							"name": containerName,
							"env": []map[string]string{
								{
									"name":  "APP_ID",
									"value": appSecret.AppId,
								},
								{
									"name":  "APP_SECRET",
									"value": appSecret.AppSecret,
								},
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		return err
	}

	switch strings.ToLower(kind) {
	case "deployment":
		_, err := r.ClientSet.
			AppsV1().
			Deployments(namespace).
			Patch(context.TODO(), name, k8stypes.StrategicMergePatchType, patchData, metav1.PatchOptions{})
		return err
	case "daemonset":
		_, err := r.ClientSet.
			AppsV1().
			DaemonSets(namespace).
			Patch(context.TODO(), name, k8stypes.StrategicMergePatchType, patchData, metav1.PatchOptions{})
		return err
	case "statefulset":
		_, err := r.ClientSet.
			AppsV1().
			StatefulSets(namespace).
			Patch(context.TODO(), name, k8stypes.StrategicMergePatchType, patchData, metav1.PatchOptions{})
		return err
	}
	return nil
}

// patchAppGroup stores APP_ID and APP_SECRET on the AppGroup CR spec.
func (r *SiteController) patchAppGroup(target *sitev1alpha1.TargetRef, appSecret *console.AppSecret) error {
	group := &appgroupv1alpha1.AppGroup{}
	if err := r.Get(context.TODO(), k8stypes.NamespacedName{Name: target.Name, Namespace: target.Namespace}, group); err != nil {
		return err
	}

	group.Spec.AppCredentials = &appgroupv1alpha1.AppCredentials{
		AppId:     appSecret.AppId,
		AppSecret: appSecret.AppSecret,
	}
	return r.Update(context.TODO(), group)
}

// createOrUpdateSecret creates or updates a Secret with APP_ID and APP_SECRET data.
func (r *SiteController) createOrUpdateSecret(target *sitev1alpha1.TargetRef, appSecret *console.AppSecret) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      target.Name,
			Namespace: target.Namespace,
		},
		Data: map[string][]byte{
			"APP_ID":     []byte(appSecret.AppId),
			"APP_SECRET": []byte(appSecret.AppSecret),
		},
		Type: corev1.SecretTypeOpaque,
	}

	existing, err := r.ClientSet.CoreV1().Secrets(target.Namespace).Get(context.TODO(), target.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = r.ClientSet.CoreV1().Secrets(target.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
			return err
		}
		return err
	}

	secret.ResourceVersion = existing.ResourceVersion
	_, err = r.ClientSet.CoreV1().Secrets(target.Namespace).Update(context.TODO(), secret, metav1.UpdateOptions{})
	return err
}

// createOrUpdateConfigMap creates or updates a ConfigMap with APP_ID and APP_SECRET data.
func (r *SiteController) createOrUpdateConfigMap(target *sitev1alpha1.TargetRef, appSecret *console.AppSecret) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      target.Name,
			Namespace: target.Namespace,
		},
		Data: map[string]string{
			"APP_ID":     appSecret.AppId,
			"APP_SECRET": appSecret.AppSecret,
		},
	}

	existing, err := r.ClientSet.CoreV1().ConfigMaps(target.Namespace).Get(context.TODO(), target.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = r.ClientSet.CoreV1().ConfigMaps(target.Namespace).Create(context.TODO(), cm, metav1.CreateOptions{})
			return err
		}
		return err
	}

	cm.ResourceVersion = existing.ResourceVersion
	_, err = r.ClientSet.CoreV1().ConfigMaps(target.Namespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}

// setPhase updates the Site status with the given phase, condition and message.
func (r *SiteController) setPhase(site *sitev1alpha1.Site, phase, reason string, conditionStatus metav1.ConditionStatus, message string) {
	setSitePhase(site, phase, reason, conditionStatus, message)
}

func setSitePhase(site *sitev1alpha1.Site, phase, reason string, conditionStatus metav1.ConditionStatus, message string) {
	now := metav1.Now()
	site.Status.Phase = phase
	site.Status.Message = message

	conditionType := "Ready"
	switch phase {
	case "AppIdReady":
		conditionType = "AppIdReady"
	case "Failed":
		conditionType = "Ready"
	}

	site.Status.Conditions = append(site.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}
