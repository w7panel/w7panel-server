package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// handlePvc converts a Longhorn PVC size increase into a resumable Controller task.
func (m *ResourceMutator) handlePvc(ctx context.Context, req admission.Request) admission.Response {
	slog.Info("处理 PersistentVolumeClaim admission 请求")

	if req.Operation != "UPDATE" {
		return admission.Allowed("无需修改 PersistentVolumeClaim")
	}
	if !facade.GetConfig().GetBool("longhorn.watch") {
		return admission.Allowed("Longhorn 扩容 Controller 未启用")
	}
	pvc := &v1.PersistentVolumeClaim{}
	if err := (m.decoder).Decode(req, pvc); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	oldPvc := &v1.PersistentVolumeClaim{}
	if err := (m.decoder).DecodeRaw(req.OldObject, oldPvc); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	oldSize := oldPvc.Spec.Resources.Requests.Storage()
	newSize := pvc.Spec.Resources.Requests.Storage()
	if newSize.Cmp(*oldSize) <= 0 {
		return admission.Allowed("PVC 容量未增加")
	}

	// Controller 在 resizing 阶段写入真实容量时必须放行，避免再次转成任务。
	if pvc.Annotations[longhorn.PVCResizeStateAnnotation] == longhorn.PVCResizeStateResizing &&
		pvc.Annotations[longhorn.PVCResizeTargetAnnotation] == newSize.String() {
		return admission.Allowed("PVC 扩容由 Controller 执行")
	}

	if oldPvc.Spec.VolumeName == "" {
		return admission.Allowed("PVC 尚未绑定")
	}
	pv, err := m.sdk.ClientSet.CoreV1().PersistentVolumes().Get(ctx, oldPvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil || pv.Spec.CSI == nil || pv.Spec.CSI.Driver != longhorn.CSIDriverName {
		return admission.Allowed("非 Longhorn PVC 扩容")
	}

	if err := preparePVCResizeRequest(oldPvc, pvc, time.Now()); err != nil {
		return admission.Denied(err.Error())
	}
	marshaled, err := json.Marshal(pvc)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaled)

}

func preparePVCResizeRequest(oldPVC, newPVC *v1.PersistentVolumeClaim, now time.Time) error {
	oldSize := oldPVC.Spec.Resources.Requests.Storage()
	target := newPVC.Spec.Resources.Requests.Storage()
	state := oldPVC.Annotations[longhorn.PVCResizeStateAnnotation]
	activeTarget := oldPVC.Annotations[longhorn.PVCResizeTargetAnnotation]
	if longhorn.IsPVCResizeActive(state) {
		if activeTarget != "" && activeTarget != target.String() {
			return &pvcResizeInProgressError{target: activeTarget}
		}
		newPVC.Spec.Resources.Requests[v1.ResourceStorage] = oldSize.DeepCopy()
		return nil
	}

	if newPVC.Annotations == nil {
		newPVC.Annotations = map[string]string{}
	}
	newPVC.Spec.Resources.Requests[v1.ResourceStorage] = oldSize.DeepCopy()
	newPVC.Annotations[longhorn.PVCResizeTargetAnnotation] = target.String()
	newPVC.Annotations[longhorn.PVCResizeStateAnnotation] = longhorn.PVCResizeStatePending
	newPVC.Annotations[longhorn.PVCResizeMessageAnnotation] = "扩容任务已提交"
	newPVC.Annotations[longhorn.PVCResizeStageAtAnnotation] = now.UTC().Format(time.RFC3339)
	delete(newPVC.Annotations, longhorn.PVCResizeNodeAnnotation)
	delete(newPVC.Annotations, longhorn.PVCResizePodsAnnotation)
	delete(newPVC.Annotations, longhorn.PVCResizeOriginallyAttachedAnnotation)
	delete(newPVC.Annotations, longhorn.PVCResizeDetachRequestedAnnotation)
	delete(newPVC.Annotations, longhorn.PVCResizeAttachRequestedAnnotation)
	return nil
}

type pvcResizeInProgressError struct{ target string }

func (e *pvcResizeInProgressError) Error() string {
	return "PVC 正在扩容到 " + e.target + "，请等待当前任务完成"
}
