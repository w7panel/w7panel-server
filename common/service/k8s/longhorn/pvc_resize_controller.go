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
	storagev1 "k8s.io/api/storage/v1"
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
	PVCResizeAttachmentTicketsAnnotation  = "storage.w7.cc/resize-attachment-tickets"
	PVCResizePodsRestartedAnnotation      = "storage.w7.cc/resize-pods-restarted"

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
	GetVolumeAttachment(string) (*longhornv1beta2.VolumeAttachment, error)
}

type pvcResizePodSnapshot struct {
	Name           string `json:"name"`
	UID            string `json:"uid,omitempty"`
	WaitForRestart bool   `json:"waitForRestart,omitempty"`
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
	attached := volume.Status.State == longhornv1beta2.VolumeStateAttached && volume.Status.CurrentNodeID != ""
	pods := make([]pvcResizePodSnapshot, 0, len(volume.Status.KubernetesStatus.WorkloadsStatus))
	for _, workload := range volume.Status.KubernetesStatus.WorkloadsStatus {
		if workload.PodName != "" {
			snapshot := pvcResizePodSnapshot{Name: workload.PodName}
			pod := &corev1.Pod{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: pvc.Namespace, Name: workload.PodName}, pod); err == nil {
				snapshot.UID = string(pod.UID)
				snapshot.WaitForRestart = metav1.GetControllerOf(pod) != nil
			}
			pods = append(pods, snapshot)
		}
	}
	podJSON, _ := json.Marshal(pods)
	tickets, err := r.captureAttachmentTickets(ctx, pvc)
	if err != nil {
		return ctrl.Result{}, err
	}
	if attached && len(tickets) == 0 {
		if r.stageTimedOut(pvc) {
			return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateFailed, "未找到扩容前 CSI 绑定凭据，未执行分离", nil)
		}
		return ctrl.Result{RequeueAfter: resizePollInterval}, nil
	}
	ticketJSON, _ := json.Marshal(tickets)
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
		PVCResizeAttachmentTicketsAnnotation:  string(ticketJSON),
		PVCResizePodsRestartedAnnotation:      "",
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
	tickets, err := r.resizeAttachmentTickets(ctx, pvc)
	if err != nil {
		return ctrl.Result{}, err
	}
	if len(tickets) == 0 {
		return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("扩容前绑定凭据为空"))
	}
	attachment, err := r.longhorn.GetVolumeAttachment(pvc.Spec.VolumeName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.restoreAttachmentTickets(pvc.Spec.VolumeName, node, tickets, attachment); err != nil {
		return ctrl.Result{}, err
	}
	if volume.Status.State == longhornv1beta2.VolumeStateAttached && volume.Status.CurrentNodeID == node && attachmentTicketsSatisfied(tickets, attachment) {
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
		if err := r.patchAnnotations(ctx, pvc, map[string]string{PVCResizeAttachRequestedAnnotation: "true"}); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: resizePollInterval}, nil
}

func (r *PVCResizeReconciler) restartPods(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (ctrl.Result, error) {
	pods, err := resizePodSnapshots(pvc.Annotations[PVCResizePodsAnnotation])
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, pvc, err)
	}
	if pvc.Annotations[PVCResizePodsRestartedAnnotation] != "true" {
		if err := r.deleteCapturedPodsInNamespace(ctx, pvc.Namespace, pods); err != nil {
			if r.stageTimedOut(pvc) {
				return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("重启关联 Pod 超时: %w", err))
			}
			return ctrl.Result{}, err
		}
		if err := r.patchAnnotations(ctx, pvc, map[string]string{PVCResizePodsRestartedAnnotation: "true"}); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: resizePollInterval}, nil
	}
	ready, err := r.restartedPodsReady(ctx, pvc.Namespace, pods)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !ready {
		if r.stageTimedOut(pvc) {
			return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待关联 Pod 恢复超时"))
		}
		return ctrl.Result{RequeueAfter: resizePollInterval}, nil
	}
	if pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] == "true" {
		volume, err := r.longhorn.GetVolume(pvc.Spec.VolumeName)
		if err != nil || volume.Status.State != longhornv1beta2.VolumeStateAttached || volume.Status.CurrentNodeID != pvc.Annotations[PVCResizeNodeAnnotation] {
			if r.stageTimedOut(pvc) {
				return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待 Longhorn 卷保持绑定超时"))
			}
			return ctrl.Result{RequeueAfter: resizePollInterval}, nil
		}
		tickets, err := r.resizeAttachmentTickets(ctx, pvc)
		if err != nil {
			return ctrl.Result{}, err
		}
		attachment, err := r.longhorn.GetVolumeAttachment(pvc.Spec.VolumeName)
		if err != nil || !attachmentTicketsSatisfied(tickets, attachment) {
			if r.stageTimedOut(pvc) {
				return ctrl.Result{}, r.fail(ctx, pvc, fmt.Errorf("等待 CSI 绑定恢复超时"))
			}
			return ctrl.Result{RequeueAfter: resizePollInterval}, nil
		}
	}
	return ctrl.Result{}, r.setStage(ctx, pvc, PVCResizeStateSucceeded, "扩容完成", map[string]string{
		PVCResizeDetachRequestedAnnotation: "",
		PVCResizeAttachRequestedAnnotation: "",
		PVCResizePodsRestartedAnnotation:   "",
	})
}

func (r *PVCResizeReconciler) fail(ctx context.Context, pvc *corev1.PersistentVolumeClaim, cause error) error {
	if pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] == "true" {
		node := pvc.Annotations[PVCResizeNodeAnnotation]
		tickets, ticketErr := r.resizeAttachmentTickets(ctx, pvc)
		if node != "" && ticketErr == nil {
			attachment, _ := r.longhorn.GetVolumeAttachment(pvc.Spec.VolumeName)
			if err := r.restoreAttachmentTickets(pvc.Spec.VolumeName, node, tickets, attachment); err != nil {
				slog.Error("restore Longhorn attachment after resize failure", "namespace", pvc.Namespace, "pvc", pvc.Name, "error", err)
			}
		}
	}
	if pvc.Annotations[PVCResizePodsRestartedAnnotation] != "true" {
		pods, _ := resizePodSnapshots(pvc.Annotations[PVCResizePodsAnnotation])
		if err := r.deleteCapturedPodsInNamespace(ctx, pvc.Namespace, pods); err != nil {
			slog.Error("restart pods after resize failure", "namespace", pvc.Namespace, "pvc", pvc.Name, "error", err)
		}
	}
	return r.setStage(ctx, pvc, PVCResizeStateFailed, cause.Error(), nil)
}

func (r *PVCResizeReconciler) captureAttachmentTickets(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (map[string]*longhornv1beta2.AttachmentTicket, error) {
	attachment, err := r.longhorn.GetVolumeAttachment(pvc.Spec.VolumeName)
	if err == nil && len(attachment.Spec.AttachmentTickets) > 0 {
		return attachment.Spec.AttachmentTickets, nil
	}
	list := &storagev1.VolumeAttachmentList{}
	if listErr := r.List(ctx, list); listErr != nil {
		return nil, listErr
	}
	tickets := map[string]*longhornv1beta2.AttachmentTicket{}
	for _, attachment := range list.Items {
		if attachment.Spec.Attacher != CSIDriverName || attachment.Spec.Source.PersistentVolumeName == nil || *attachment.Spec.Source.PersistentVolumeName != pvc.Spec.VolumeName {
			continue
		}
		tickets[attachment.Name] = &longhornv1beta2.AttachmentTicket{
			ID: attachment.Name, Type: longhornv1beta2.AttacherTypeCSIAttacher, NodeID: attachment.Spec.NodeName,
			Parameters: map[string]string{longhornv1beta2.AttachmentParameterDisableFrontend: "false", longhornv1beta2.AttachmentParameterLastAttachedBy: ""},
		}
	}
	return tickets, nil
}

func (r *PVCResizeReconciler) resizeAttachmentTickets(ctx context.Context, pvc *corev1.PersistentVolumeClaim) (map[string]*longhornv1beta2.AttachmentTicket, error) {
	tickets := map[string]*longhornv1beta2.AttachmentTicket{}
	if raw := pvc.Annotations[PVCResizeAttachmentTicketsAnnotation]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &tickets); err != nil {
			return nil, fmt.Errorf("解析扩容前绑定凭据失败: %w", err)
		}
	}
	if len(tickets) > 0 {
		return tickets, nil
	}
	tickets, err := r.captureAttachmentTickets(ctx, pvc)
	if err != nil || len(tickets) == 0 {
		return tickets, err
	}
	raw, _ := json.Marshal(tickets)
	if err := r.patchAnnotations(ctx, pvc, map[string]string{PVCResizeAttachmentTicketsAnnotation: string(raw)}); err != nil {
		return nil, err
	}
	return tickets, nil
}

func (r *PVCResizeReconciler) restoreAttachmentTickets(volumeName, fallbackNode string, tickets map[string]*longhornv1beta2.AttachmentTicket, current *longhornv1beta2.VolumeAttachment) error {
	for id, ticket := range tickets {
		if current != nil && current.Spec.AttachmentTickets[id] != nil {
			continue
		}
		node := ticket.NodeID
		if node == "" {
			node = fallbackNode
		}
		disableFrontend := ticket.Parameters[longhornv1beta2.AttachmentParameterDisableFrontend]
		if disableFrontend == "" {
			disableFrontend = "false"
		}
		if err := r.attachVolume(volumeName, node, id, ticket.Parameters[longhornv1beta2.AttachmentParameterLastAttachedBy], string(ticket.Type), disableFrontend); err != nil {
			return err
		}
	}
	return nil
}

func attachmentTicketsSatisfied(tickets map[string]*longhornv1beta2.AttachmentTicket, attachment *longhornv1beta2.VolumeAttachment) bool {
	if len(tickets) == 0 || attachment == nil {
		return false
	}
	for id := range tickets {
		if !longhornv1beta2.IsAttachmentTicketSatisfied(id, attachment) {
			return false
		}
	}
	return true
}

func resizePodSnapshots(raw string) ([]pvcResizePodSnapshot, error) {
	if raw == "" {
		return nil, nil
	}
	var legacy []string
	if err := json.Unmarshal([]byte(raw), &legacy); err == nil {
		snapshots := make([]pvcResizePodSnapshot, 0, len(legacy))
		for _, name := range legacy {
			snapshots = append(snapshots, pvcResizePodSnapshot{Name: name, WaitForRestart: true})
		}
		return snapshots, nil
	}
	var snapshots []pvcResizePodSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshots); err != nil {
		return nil, fmt.Errorf("解析关联 Pod 列表失败: %w", err)
	}
	return snapshots, nil
}

func (r *PVCResizeReconciler) deleteCapturedPodsInNamespace(ctx context.Context, namespace string, pods []pvcResizePodSnapshot) error {
	for _, pod := range pods {
		err := r.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: pod.Name, Namespace: namespace}})
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *PVCResizeReconciler) restartedPodsReady(ctx context.Context, namespace string, pods []pvcResizePodSnapshot) (bool, error) {
	for _, snapshot := range pods {
		if !snapshot.WaitForRestart {
			continue
		}
		pod := &corev1.Pod{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: snapshot.Name}, pod); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if snapshot.UID != "" && string(pod.UID) == snapshot.UID {
			return false, nil
		}
		ready := false
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return false, nil
		}
	}
	return true, nil
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
