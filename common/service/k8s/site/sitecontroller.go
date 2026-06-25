package site

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	sitev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/site/v1alpha1"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SetupSiteController sets up the Site controller with the manager.
func SetupSiteController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	// Register Site types with the manager's scheme
	if err := sitev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
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

	// Fetch the Site instance
	site := &sitev1alpha1.Site{}
	if err := r.Get(ctx, req.NamespacedName, site); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Site")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	// Check if the Site is being deleted
	if !site.DeletionTimestamp.IsZero() {
		logger.Info("Site is being deleted", "name", req.Name)
		return ctrl.Result{}, nil
	}

	// If already registered, skip
	if site.Spec.AppId != "" && site.Spec.AppSecret != "" {
		logger.Info("Site already registered, skipping", "appId", site.Spec.AppId)
		return ctrl.Result{}, nil
	}

	// In child agent mode, sync to root panel via HTTP instead of processing locally
	if helper.IsChildAgent() {
		logger.Info("Child agent mode, syncing site to root panel")
		if err := k3k.SyncSiteHttp(site); err != nil {
			logger.Error(err, "Failed to sync site to root panel")
			r.updateSiteStatus(site, metav1.ConditionFalse, "SyncFailed", fmt.Sprintf("sync site error: %s", err.Error()))
			if err := r.Status().Update(ctx, site); err != nil {
				logger.Error(err, "Failed to update site status")
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		logger.Info("Site synced to root panel successfully")
		r.updateSiteStatus(site, metav1.ConditionTrue, "Synced", "site synced to root panel")
		if err := r.Status().Update(ctx, site); err != nil {
			logger.Error(err, "Failed to update site status")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}
	var secret *console.AppSecret
	if !helper.IsChildAgent() {
		// Register the site via ZPK
		sec, err := console.RegisterSiteZpk(site.Spec.Host, site.Spec.SiteIdentifier)
		if err != nil {
			logger.Error(err, "Failed to register site via ZPK")
			r.updateSiteStatus(site, metav1.ConditionFalse, "RegistrationFailed", fmt.Sprintf("register site zpk error: %s", err.Error()))
			if err := r.Status().Update(ctx, site); err != nil {
				logger.Error(err, "Failed to update site status")
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
			return ctrl.Result{}, nil
		}
		logger.Info("Site registered successfully", "appId", sec.AppId)
		secret = sec
	}

	// Patch the target resource (Deployment / DaemonSet / StatefulSet) with AppId/AppSecret
	err := r.patchTargetResource(site.Name, secret, site.Spec.Target)
	if err != nil {
		logger.Error(err, "Failed to patch target resource")
		r.updateSiteStatus(site, metav1.ConditionFalse, "PatchFailed", fmt.Sprintf("patch target error: %s", err.Error()))
		if err := r.Status().Update(ctx, site); err != nil {
			logger.Error(err, "Failed to update site status")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	logger.Info("Target resource patched successfully")

	// Update Site spec with the registered credentials
	site.Spec.AppId = secret.AppId
	site.Spec.AppSecret = secret.AppSecret
	if err := r.Update(ctx, site); err != nil {
		logger.Error(err, "Failed to update site spec")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	// Update status to Registered
	r.updateSiteStatus(site, metav1.ConditionTrue, "Registered", "site registered and target patched successfully")
	if err := r.Status().Update(ctx, site); err != nil {
		logger.Error(err, "Failed to update site status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

// patchTargetResource injects APP_ID and APP_SECRET env vars into the target workload
// (Deployment, DaemonSet, or StatefulSet) using a strategic merge patch.
func (r *SiteController) patchTargetResource(siteName string, appSecret *console.AppSecret, target sitev1alpha1.TargetRef) error {
	patchData := fmt.Sprintf(`{
		"spec": {
			"template": {
				"spec": {
					"containers": [
						{
							"name": "%s",
							"env": [
								{
									"name": "APP_ID",
									"value": "%s"
								},
								{
									"name": "APP_SECRET",
									"value": "%s"
								}
							]
						}
					]
				}
			}
		}
	}`, target.ContainerName, appSecret.AppId, appSecret.AppSecret)

	switch target.Kind {
	case "Deployment":
		_, err := r.ClientSet.
			AppsV1().
			Deployments(target.Namespace).
			Patch(context.TODO(), siteName, k8stypes.StrategicMergePatchType, []byte(patchData), metav1.PatchOptions{})
		return err
	case "DaemonSet":
		_, err := r.ClientSet.
			AppsV1().
			DaemonSets(target.Namespace).
			Patch(context.TODO(), siteName, k8stypes.StrategicMergePatchType, []byte(patchData), metav1.PatchOptions{})
		return err
	case "StatefulSet":
		_, err := r.ClientSet.
			AppsV1().
			StatefulSets(target.Namespace).
			Patch(context.TODO(), siteName, k8stypes.StrategicMergePatchType, []byte(patchData), metav1.PatchOptions{})
		return err
	default:
		return fmt.Errorf("unsupported target kind %q (supported: Deployment, DaemonSet, StatefulSet)", target.Kind)
	}
}

// updateSiteStatus sets the Site status fields (phase, conditions, message).
func (r *SiteController) updateSiteStatus(site *sitev1alpha1.Site, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	phase := "Registered"
	if status == metav1.ConditionFalse {
		phase = "Failed"
	}

	site.Status.Phase = phase
	site.Status.Message = message
	site.Status.Conditions = append(site.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
	})
}
