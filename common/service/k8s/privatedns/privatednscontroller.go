package privatedns

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/w7panel/w7panel/common/service/coredns"
	"github.com/w7panel/w7panel/common/service/k8s"
	privatednsv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/privatedns/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const PrivateDNSFinalizerName = "privatedns.w7panel.w7.com/finalizer"

func SetupPrivateDNSController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	if err := privatednsv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	r := &PrivateDNSController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&privatednsv1alpha1.PrivateDNS{}).
		Complete(r)
}

type PrivateDNSController struct {
	client.Client
	Scheme *runtime.Scheme
	*k8s.Sdk
}

func (r *PrivateDNSController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("PrivateDNS reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
		)
	}
	return result, err
}

func (r *PrivateDNSController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling PrivateDNS", "name", req.Name)

	privateDNS := &privatednsv1alpha1.PrivateDNS{}
	if err := r.Get(ctx, req.NamespacedName, privateDNS); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get PrivateDNS")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if !privateDNS.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, privateDNS)
	}

	if !controllerutil.ContainsFinalizer(privateDNS, PrivateDNSFinalizerName) {
		controllerutil.AddFinalizer(privateDNS, PrivateDNSFinalizerName)
		if err := r.Update(ctx, privateDNS); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	records := make([]coredns.Record, 0, len(privateDNS.Spec.Records))
	for _, record := range privateDNS.Spec.Records {
		records = append(records, coredns.Record{
			ID:         record.ID,
			Name:       record.Name,
			Type:       record.Type,
			Value:      record.Value,
			TTL:        record.TTL,
			MXPriority: record.MXPriority,
		})
	}

	zone, normalizedRecords, err := coredns.NewServiceWithSdk(r.Sdk).ApplyZoneRecords(ctx, privateDNS.Spec.Domain, records)
	if err != nil {
		logger.Error(err, "Failed to apply private DNS zone")
		return r.updateStatus(ctx, privateDNS, "Failed", "ApplyFailed", err.Error(), 0, metav1.ConditionFalse)
	}

	message := fmt.Sprintf("private DNS zone %s applied", zone.Domain)
	return r.updateStatus(ctx, privateDNS, "Ready", "Applied", message, len(normalizedRecords), metav1.ConditionTrue)
}

func (r *PrivateDNSController) handleDeletion(ctx context.Context, privateDNS *privatednsv1alpha1.PrivateDNS) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if controllerutil.ContainsFinalizer(privateDNS, PrivateDNSFinalizerName) {
		if privateDNS.Spec.Domain != "" {
			if err := coredns.NewServiceWithSdk(r.Sdk).DeleteZone(ctx, privateDNS.Spec.Domain); err != nil {
				logger.Error(err, "Failed to delete private DNS zone")
				return ctrl.Result{RequeueAfter: time.Minute}, nil
			}
		}
		controllerutil.RemoveFinalizer(privateDNS, PrivateDNSFinalizerName)
		if err := r.Update(ctx, privateDNS); err != nil {
			logger.Error(err, "Failed to remove finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *PrivateDNSController) updateStatus(ctx context.Context, privateDNS *privatednsv1alpha1.PrivateDNS, phase, reason, message string, recordCount int, conditionStatus metav1.ConditionStatus) (ctrl.Result, error) {
	privateDNS.Status.Phase = phase
	privateDNS.Status.Message = message
	privateDNS.Status.RecordCount = recordCount
	privateDNS.Status.ObservedGeneration = privateDNS.Generation
	meta.SetStatusCondition(&privateDNS.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             conditionStatus,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: privateDNS.Generation,
		LastTransitionTime: metav1.Now(),
	})
	if err := r.Status().Update(ctx, privateDNS); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update PrivateDNS status")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	return ctrl.Result{}, nil
}
