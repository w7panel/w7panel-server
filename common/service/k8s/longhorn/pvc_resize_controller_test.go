package longhorn

import (
	"context"
	"testing"
	"time"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeResizeLonghornClient struct {
	volume  *longhornv1beta2.Volume
	engines *longhornv1beta2.EngineList
}

func (f *fakeResizeLonghornClient) GetVolume(string) (*longhornv1beta2.Volume, error) {
	return f.volume.DeepCopy(), nil
}

func (f *fakeResizeLonghornClient) GetEngineList() (*longhornv1beta2.EngineList, error) {
	return f.engines.DeepCopy(), nil
}

func resizeControllerTestPVC(state string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data",
			Namespace: "default",
			Annotations: map[string]string{
				PVCResizeTargetAnnotation:  "20Gi",
				PVCResizeStateAnnotation:   state,
				PVCResizeStageAtAnnotation: time.Now().UTC().Format(time.RFC3339),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			VolumeName: "pvc-volume",
			Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			}},
		},
	}
}

func newResizeTestReconciler(t *testing.T, objects ...client.Object) (*PVCResizeReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	r := &PVCResizeReconciler{
		Client: cl,
		longhorn: &fakeResizeLonghornClient{
			volume:  &longhornv1beta2.Volume{},
			engines: &longhornv1beta2.EngineList{},
		},
		detachVolume: func(string, string, bool) error { return nil },
		attachVolume: func(string, string, string, string, string, string) error { return nil },
		now:          time.Now,
	}
	return r, cl
}

func TestPVCResizePrepareCapturesNodeAndPods(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStatePending)
	r, cl := newResizeTestReconciler(t, pvc)
	volume := &longhornv1beta2.Volume{
		Status: longhornv1beta2.VolumeStatus{
			State:         longhornv1beta2.VolumeStateAttached,
			CurrentNodeID: "server1",
			KubernetesStatus: longhornv1beta2.KubernetesStatus{WorkloadsStatus: []longhornv1beta2.WorkloadStatus{
				{PodName: "mysql-0"},
			}},
		},
	}
	if _, err := r.prepare(context.Background(), pvc, volume); err != nil {
		t.Fatal(err)
	}
	got := &corev1.PersistentVolumeClaim{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pvc), got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[PVCResizeStateAnnotation] != PVCResizeStateDetaching {
		t.Fatalf("expected detaching, got %q", got.Annotations[PVCResizeStateAnnotation])
	}
	if got.Annotations[PVCResizeNodeAnnotation] != "server1" {
		t.Fatalf("expected original node server1, got %q", got.Annotations[PVCResizeNodeAnnotation])
	}
	if got.Annotations[PVCResizePodsAnnotation] != `["mysql-0"]` {
		t.Fatalf("unexpected pod snapshot %q", got.Annotations[PVCResizePodsAnnotation])
	}
}

func TestPVCResizeRestartDeletesPodsLastAndCompletes(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStateRestarting)
	pvc.Annotations[PVCResizePodsAnnotation] = `["mysql-0"]`
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-0", Namespace: "default"}}
	detachedTemporaryTicket := false
	r, cl := newResizeTestReconciler(t, pvc, pod)
	r.detachVolume = func(_ string, attachmentID string, force bool) error {
		detachedTemporaryTicket = attachmentID == resizeAttachmentID && !force
		return nil
	}
	pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] = "true"
	if _, err := r.restartPods(context.Background(), pvc); err != nil {
		t.Fatal(err)
	}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pod), &corev1.Pod{}); !apierrors.IsNotFound(err) {
		t.Fatalf("expected pod to be deleted, got %v", err)
	}
	got := &corev1.PersistentVolumeClaim{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pvc), got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[PVCResizeStateAnnotation] != PVCResizeStateSucceeded {
		t.Fatalf("expected succeeded, got %q", got.Annotations[PVCResizeStateAnnotation])
	}
	if !detachedTemporaryTicket {
		t.Fatal("expected temporary resize attachment to be removed")
	}
}
