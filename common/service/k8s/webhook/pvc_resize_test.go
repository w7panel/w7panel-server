package webhook

import (
	"testing"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func resizeTestPVC(size string, annotations map[string]string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Annotations: annotations},
		Spec: corev1.PersistentVolumeClaimSpec{Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(size)},
		}},
	}
}

func TestPreparePVCResizeRequestCreatesPendingTask(t *testing.T) {
	now := time.Date(2026, 8, 5, 1, 2, 3, 0, time.UTC)
	oldPVC := resizeTestPVC("10Gi", nil)
	newPVC := resizeTestPVC("20Gi", nil)

	if err := preparePVCResizeRequest(oldPVC, newPVC, now); err != nil {
		t.Fatal(err)
	}
	if got := newPVC.Spec.Resources.Requests[corev1.ResourceStorage]; got.Cmp(resource.MustParse("10Gi")) != 0 {
		t.Fatalf("expected admitted size to remain 10Gi, got %s", got.String())
	}
	if got := newPVC.Annotations[longhorn.PVCResizeTargetAnnotation]; got != "20Gi" {
		t.Fatalf("expected target 20Gi, got %q", got)
	}
	if got := newPVC.Annotations[longhorn.PVCResizeStateAnnotation]; got != longhorn.PVCResizeStatePending {
		t.Fatalf("expected pending state, got %q", got)
	}
}

func TestPreparePVCResizeRequestKeepsActiveTask(t *testing.T) {
	annotations := map[string]string{
		longhorn.PVCResizeTargetAnnotation: "20Gi",
		longhorn.PVCResizeStateAnnotation:  longhorn.PVCResizeStateDetaching,
		longhorn.PVCResizeNodeAnnotation:   "server1",
	}
	oldPVC := resizeTestPVC("10Gi", annotations)
	newPVC := resizeTestPVC("20Gi", map[string]string{
		longhorn.PVCResizeTargetAnnotation: "20Gi",
		longhorn.PVCResizeStateAnnotation:  longhorn.PVCResizeStateDetaching,
		longhorn.PVCResizeNodeAnnotation:   "server1",
	})

	if err := preparePVCResizeRequest(oldPVC, newPVC, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := newPVC.Annotations[longhorn.PVCResizeStateAnnotation]; got != longhorn.PVCResizeStateDetaching {
		t.Fatalf("expected detaching state to be preserved, got %q", got)
	}
	if got := newPVC.Annotations[longhorn.PVCResizeNodeAnnotation]; got != "server1" {
		t.Fatalf("expected captured node to be preserved, got %q", got)
	}
}

func TestPreparePVCResizeRequestRejectsDifferentActiveTarget(t *testing.T) {
	oldPVC := resizeTestPVC("10Gi", map[string]string{
		longhorn.PVCResizeTargetAnnotation: "20Gi",
		longhorn.PVCResizeStateAnnotation:  longhorn.PVCResizeStateAttaching,
	})
	newPVC := resizeTestPVC("30Gi", nil)
	if err := preparePVCResizeRequest(oldPVC, newPVC, time.Now()); err == nil {
		t.Fatal("expected a different target to be rejected while resize is active")
	}
}
