package longhorn

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	CSIDriverName = "driver.longhorn.io"

	PVCResizeTargetAnnotation             = "storage.w7.cc/resize-target"
	PVCResizeStateAnnotation              = "storage.w7.cc/resize-state"
	PVCResizeMessageAnnotation            = "storage.w7.cc/resize-message"
	PVCResizeStageAtAnnotation            = "storage.w7.cc/resize-stage-at"
	PVCResizeNodeAnnotation               = "storage.w7.cc/resize-node"
	PVCResizePodsAnnotation               = "storage.w7.cc/resize-pods"
	PVCResizeOriginallyAttachedAnnotation = "storage.w7.cc/resize-originally-attached"
	PVCResizeDetachRequestedAnnotation    = "storage.w7.cc/resize-detach-requested"
	PVCResizeAttachRequestedAnnotation    = "storage.w7.cc/resize-attach-requested"

	PVCResizeStatePending    = "pending"
	PVCResizeStateDetaching  = "detaching"
	PVCResizeStateResizing   = "resizing"
	PVCResizeStateAttaching  = "attaching"
	PVCResizeStateRestarting = "restarting"
	PVCResizeStateSucceeded  = "succeeded"
	PVCResizeStateFailed     = "failed"

	resizeAttachmentID = "w7panel-resize"
	resizePollInterval = 2 * time.Second
	resizeStageTimeout = 10 * time.Minute
)

type pvcResizeLonghornClient interface {
	GetVolume(string) (*longhornv1beta2.Volume, error)
	GetEngineList() (*longhornv1beta2.EngineList, error)
}

type PVCResizeReconciler struct {
	client.Client
	longhorn     pvcResizeLonghornClient
	detachVolume func(string, string, bool) error
	attachVolume func(string, string, string, string, string, string) error
	now          func() time.Time
}

func IsPVCResizeActive(state string) bool {
	switch state {
	case PVCResizeStatePending, PVCResizeStateDetaching, PVCResizeStateResizing,
		PVCResizeStateAttaching, PVCResizeStateRestarting:
		return true
	default:
		return false
	}
}

func SetupPVCResizeController(mgr manager.Manager, sdk *k8s.Sdk) error {
	lh, err := NewLonghornClient(sdk)
	if err != nil {
		return err
	}
	reconciler := &PVCResizeReconciler{
		Client:       mgr.GetClient(),
		longhorn:     lh,
		detachVolume: LonghornVolumeDetach,
		attachVolume: LonghornVolumeAttach,
		now:          time.Now,
	}
	resizePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object != nil && IsPVCResizeActive(e.Object.GetAnnotations()[PVCResizeStateAnnotation])
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew != nil && IsPVCResizeActive(e.ObjectNew.GetAnnotations()[PVCResizeStateAnnotation])
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.PersistentVolumeClaim{}, builder.WithPredicates(resizePredicate)).
		Complete(reconciler)
}

func (r *PVCResizeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, req.NamespacedName, pvc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	state := pvc.Annotations[PVCResizeStateAnnotation]
	if !IsPVCResizeActive(state) {
		return ctrl.Result{}, nil
	}

	target, err := resource.ParseQuantity(pvc.Annotations[PVCResizeTargetAnnotation])
	if err != nil || target.Sign() <= 0 {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("无效的目标容量 %q", pvc.Annotations[PVCResizeTargetAnnotation]))
	}
	if pvc.Spec.VolumeName == "" {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("PVC 尚未绑定 PV"))
	}
	volume, err := r.longhorn.GetVolume(pvc.Spec.VolumeName)
	if err != nil {
		if r.stageTimedOut(pvc) {
			return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("读取 Longhorn 卷状态超时: %w", err))
		}
		return ctrl.Result{}, err
	}

	switch state {
	case PVCResizeStatePending:
		return r.prepare(ctx, pvc, volume)
	case PVCResizeStateDetaching:
		return r.detach(ctx, pvc, volume)
	case PVCResizeStateResizing:
		return r.resize(ctx, pvc, volume, target)
	case PVCResizeStateAttaching:
		return r.attach(ctx, pvc, volume, target)
	case PVCResizeStateRestarting:
		return r.restartPods(ctx, pvc)
	default:
		return ctrl.Result{}, nil
	}
}

func (r *PVCResizeReconciler) prepare(ctx context.Context, pvc *corev1.PersistentVolumeClaim, volume *longhornv1beta2.Volume) (ctrl.Result, error) {
	pods := make([]string, 0, len(volume.Status.KubernetesStatus.WorkloadsStatus))
	for _, workload := range volume.Status.KubernetesStatus.WorkloadsStatus {
		if workload.PodName != "" {
			pods = append(pods, workload.PodName)
		}
	}
	podJSON, _ := json.Marshal(pods)
	attached := volume.Status.State == longhornv1beta2.VolumeStateAttached && volume.Status.CurrentNodeID != ""
	next := PVCResizeStateResizing
	message := "正在提交容量变更"
	if attached {
		next = PVCResizeStateDetaching
		message = "正在分离存储卷"
	}
	return ctrl.Result{}, r.setStage(ctx, pvc, next, message, map[string]string{
		PVCResizeNodeAnnotation:               volume.Status.CurrentNodeID,
		PVCResizePodsAnnotation:               string(podJSON),
		PVCResizeOriginallyAttachedAnnotation: fmt.Sprintf("%t", attached),
	})
}

func (r *PVCResizeReconciler) detach(ctx context.Context, pvc *corev1.PersistentVolumeClaim, volume *longhornv1beta2.Volume) (ctrl.Result, error) {
	if volume.Status.State == longhornv1beta2.VolumeStateDetached {
		return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateResizing, "正在提交容量变更", nil)
	}
	if r.stageTimedOut(pvc) {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待 Longhorn 卷分离超时"))
	}
	if pvc.Annotations[PVCResizeDetachRequestedAnnotation] != "true" {
		if err := r.detachVolume(pvc.Spec.VolumeName, resizeAttachmentID, true); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.patchAnnotations(ctx, pvc, map[string]string{PVCResizeDetachRequestedAnnotation: "true"}); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: resizePollInterval}, nil
}

func (r *PVCResizeReconciler) resize(ctx context.Context, pvc *corev1.PersistentVolumeClaim, volume *longhornv1beta2.Volume, target resource.Quantity) (ctrl.Result, error) {
	if r.stageTimedOut(pvc) {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待 PVC 和 Longhorn 接收扩容请求超时"))
	}
	current := pvc.Spec.Resources.Requests.Storage()
	if current.Cmp(target) < 0 {
		base := pvc.DeepCopy()
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = target.DeepCopy()
		pvc.Annotations[PVCResizeMessageAnnotation] = "正在扩容存储卷"
		if err := r.Patch(ctx, pvc, client.MergeFrom(base)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: resizePollInterval}, nil
	}
	if volume.Spec.Size < target.Value() {
		return ctrl.Result{RequeueAfter: resizePollInterval}, nil
	}
	if pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] != "true" {
		return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateRestarting, "正在重启关联 Pod", nil)
	}
	return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateAttaching, "正在绑定存储卷", nil)
}

func (r *PVCResizeReconciler) attach(ctx context.Context, pvc *corev1.PersistentVolumeClaim, volume *longhornv1beta2.Volume, target resource.Quantity) (ctrl.Result, error) {
	node := pvc.Annotations[PVCResizeNodeAnnotation]
	if node == "" {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("扩容前绑定节点为空"))
	}
	if volume.Status.State == longhornv1beta2.VolumeStateAttached && volume.Status.CurrentNodeID == node {
		engines, err := r.longhorn.GetEngineList()
		if err != nil {
			return ctrl.Result{}, err
		}
		engine := selectVolumeEngine(pvc.Spec.VolumeName, engines)
		if engine != nil && engine.Status.CurrentSize >= target.Value() {
			return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateRestarting, "正在重启关联 Pod", nil)
		}
	}
	if r.stageTimedOut(pvc) {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待 Longhorn 卷绑定或容量同步超时"))
	}
	if pvc.Annotations[PVCResizeAttachRequestedAnnotation] != "true" {
		if err := r.attachVolume(pvc.Spec.VolumeName, node, resizeAttachmentID, "w7panel", "", "false"); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.patchAnnotations(ctx, pvc, map[string]string{PVCResizeAttachRequestedAnnotation: "true"}); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: resizePollInterval}, nil
}

func (r *PVCResizeReconciler) restartPods(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (ctrl.Result, error) {
	if err := r.deleteCapturedPods(ctx, pvc); err != nil {
		if r.stageTimedOut(pvc) {
			return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("重启关联 Pod 超时: %w", err))
		}
		return ctrl.Result{}, err
	}
	if pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] == "true" {
		if err := r.detachVolume(pvc.Spec.VolumeName, resizeAttachmentID, false); err != nil {
			slog.Warn("remove temporary resize attachment failed", "namespace", pvc.Namespace, "pvc", pvc.Name, "error", err)
		}
	}
	return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateSucceeded, "扩容完成", map[string]string{
		PVCResizeDetachRequestedAnnotation: "",
		PVCResizeAttachRequestedAnnotation: "",
	})
}

func (r *PVCResizeReconciler) deleteCapturedPods(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	var pods []string
	if raw := pvc.Annotations[PVCResizePodsAnnotation]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &pods); err != nil {
			return fmt.Errorf("解析关联 Pod 列表失败: %w", err)
		}
	}
	for _, podName := range pods {
		err := r.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: pvc.Namespace}})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *PVCResizeReconciler) fail(ctx context.Context, pvc *corev1.PersistentVolumeClaim, cause error) error {
	if pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] == "true" {
		node := pvc.Annotations[PVCResizeNodeAnnotation]
		if node != "" {
			if err := r.attachVolume(pvc.Spec.VolumeName, node, resizeAttachmentID, "w7panel-recovery", "", "false"); err != nil {
				slog.Error("restore Longhorn attachment after resize failure", "namespace", pvc.Namespace, "pvc", pvc.Name, "error", err)
			}
		}
	}
	if err := r.deleteCapturedPods(ctx, pvc); err != nil {
		slog.Error("restart pods after resize failure", "namespace", pvc.Namespace, "pvc", pvc.Name, "error", err)
	}
	return r.setStage(ctx, pvc, PVCResizeStateFailed, cause.Error(), nil)
}

func (r *PVCResizeReconciler) setStage(ctx context.Context, pvc *corev1.PersistentVolumeClaim, state, message string, extra map[string]string) error {
	values := map[string]string{
		PVCResizeStateAnnotation:   state,
		PVCResizeMessageAnnotation: message,
		PVCResizeStageAtAnnotation: r.now().UTC().Format(time.RFC3339),
	}
	for key, value := range extra {
		values[key] = value
	}
	return r.patchAnnotations(ctx, pvc, values)
}

func (r *PVCResizeReconciler) patchAnnotations(ctx context.Context, pvc *corev1.PersistentVolumeClaim, values map[string]string) error {
	base := pvc.DeepCopy()
	if pvc.Annotations == nil {
		pvc.Annotations = map[string]string{}
	}
	for key, value := range values {
		if value == "" {
			delete(pvc.Annotations, key)
			continue
		}
		pvc.Annotations[key] = value
	}
	return r.Patch(ctx, pvc, client.MergeFrom(base))
}

func (r *PVCResizeReconciler) stageTimedOut(pvc *corev1.PersistentVolumeClaim) bool {
	stageAt, err := time.Parse(time.RFC3339, strings.TrimSpace(pvc.Annotations[PVCResizeStageAtAnnotation]))
	return err == nil && r.now().Sub(stageAt) > resizeStageTimeout
}
