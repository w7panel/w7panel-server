package longhorn

import (
	"context"
	"testing"
	"time"

	longhornv1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeResizeLonghornClient struct {
	volume     *longhornv1beta2.Volume
	engines    *longhornv1beta2.EngineList
	attachment *longhornv1beta2.VolumeAttachment
}

func (f *fakeResizeLonghornClient) GetVolume(string) (*longhornv1beta2.Volume, error) {
	return f.volume.DeepCopy(), nil
}

func (f *fakeResizeLonghornClient) GetEngineList() (*longhornv1beta2.EngineList, error) {
	return f.engines.DeepCopy(), nil
}

func (f *fakeResizeLonghornClient) GetVolumeAttachment(string) (*longhornv1beta2.VolumeAttachment, error) {
	return f.attachment.DeepCopy(), nil
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
	if err := storagev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
	r := &PVCResizeReconciler{
		Client: cl,
		longhorn: &fakeResizeLonghornClient{
			volume:     &longhornv1beta2.Volume{},
			engines:    &longhornv1beta2.EngineList{},
			attachment: &longhornv1beta2.VolumeAttachment{},
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
	r.longhorn.(*fakeResizeLonghornClient).attachment = &longhornv1beta2.VolumeAttachment{
		Spec: longhornv1beta2.VolumeAttachmentSpec{AttachmentTickets: map[string]*longhornv1beta2.AttachmentTicket{
			"csi-test": {ID: "csi-test", Type: longhornv1beta2.AttacherTypeCSIAttacher, NodeID: "server1"},
		}},
	}
	r.longhorn.(*fakeResizeLonghornClient).volume = &longhornv1beta2.Volume{
		Status: longhornv1beta2.VolumeStatus{State: longhornv1beta2.VolumeStateAttached, CurrentNodeID: "server1"},
	}
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
	if got.Annotations[PVCResizePodsAnnotation] != `[{"name":"mysql-0"}]` {
		t.Fatalf("unexpected pod snapshot %q", got.Annotations[PVCResizePodsAnnotation])
	}
}

func TestPVCResizeRestartCompletesAfterPodDeleteAndKeepsCSITicket(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStateRestarting)
	pvc.Annotations[PVCResizePodsAnnotation] = `[{"name":"mysql-0","uid":"old-uid","waitForRestart":true}]`
	pvc.Annotations[PVCResizeOriginallyAttachedAnnotation] = "true"
	pvc.Annotations[PVCResizeNodeAnnotation] = "server1"
	pvc.Annotations[PVCResizeAttachmentTicketsAnnotation] = `{"csi-test":{"id":"csi-test","type":"csi-attacher","nodeID":"server1","parameters":{"disableFrontend":"false"}}}`
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "mysql-0", Namespace: "default", UID: "old-uid"}}
	r, cl := newResizeTestReconciler(t, pvc, pod)
	r.longhorn.(*fakeResizeLonghornClient).attachment = &longhornv1beta2.VolumeAttachment{
		Spec: longhornv1beta2.VolumeAttachmentSpec{AttachmentTickets: map[string]*longhornv1beta2.AttachmentTicket{
			"csi-test": {ID: "csi-test", Generation: 1},
		}},
		Status: longhornv1beta2.VolumeAttachmentStatus{AttachmentTicketStatuses: map[string]*longhornv1beta2.AttachmentTicketStatus{
			"csi-test": {ID: "csi-test", Generation: 1, Satisfied: true},
		}},
	}
	r.longhorn.(*fakeResizeLonghornClient).volume = &longhornv1beta2.Volume{
		Status: longhornv1beta2.VolumeStatus{State: longhornv1beta2.VolumeStateAttached, CurrentNodeID: "server1"},
	}
	if _, err := r.restartPods(context.Background(), pvc); err != nil {
		t.Fatal(err)
	}
	got := &corev1.PersistentVolumeClaim{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(pvc), got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[PVCResizeStateAnnotation] != PVCResizeStateSucceeded {
		t.Fatalf("expected succeeded, got %q", got.Annotations[PVCResizeStateAnnotation])
	}
	if got.Annotations[PVCResizePodsRestartedAnnotation] != "" {
		t.Fatal("expected pod restart marker to be cleared")
	}
}

func TestPVCResizeRestartAcceptsReadyDeploymentReplacementWithNewName(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStateRestarting)
	pvc.Annotations[PVCResizePodsAnnotation] = `[{"name":"app-7b9c8d6f4f-old","uid":"old-pod","controllerUID":"replica-set","waitForRestart":true}]`
	pvc.Annotations[PVCResizePodsRestartedAnnotation] = "true"
	old := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "app-7b9c8d6f4f-old", Namespace: "default", UID: "old-pod"}}
	controller := true
	replacement := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app-7b9c8d6f4f-new", Namespace: "default", UID: "new-pod", OwnerReferences: []metav1.OwnerReference{{UID: "replica-set", Controller: &controller}}},
		Status:     corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
	}
	r, cl := newResizeTestReconciler(t, pvc, old, replacement)
	if err := cl.Delete(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	ready, err := r.restartedPodsReady(context.Background(), "default", []pvcResizePodSnapshot{{Name: old.Name, UID: string(old.UID), ControllerUID: "replica-set", WaitForRestart: true}})
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("expected ready replacement Pod with a new Deployment name")
	}
}

func TestPVCResizeCapturesCSITicketFromKubernetesVolumeAttachment(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStatePending)
	pvName := pvc.Spec.VolumeName
	attachment := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: "csi-test"},
		Spec: storagev1.VolumeAttachmentSpec{
			Attacher: CSIDriverName,
			NodeName: "server1",
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: &pvName},
		},
	}
	r, _ := newResizeTestReconciler(t, pvc, attachment)
	tickets, err := r.captureAttachmentTickets(context.Background(), pvc)
	if err != nil {
		t.Fatal(err)
	}
	ticket := tickets["csi-test"]
	if ticket == nil || ticket.Type != longhornv1beta2.AttacherTypeCSIAttacher || ticket.NodeID != "server1" {
		t.Fatalf("unexpected restored CSI ticket %#v", ticket)
	}
}

func TestPVCResizeRestoresOriginalAttachmentTicket(t *testing.T) {
	pvc := resizeControllerTestPVC(PVCResizeStateAttaching)
	r, _ := newResizeTestReconciler(t, pvc)
	var gotID, gotType, gotNode string
	r.attachVolume = func(_ string, node string, id string, _ string, ticketType string, _ string) error {
		gotID, gotType, gotNode = id, ticketType, node
		return nil
	}
	tickets := map[string]*longhornv1beta2.AttachmentTicket{
		"csi-test": {ID: "csi-test", Type: longhornv1beta2.AttacherTypeCSIAttacher, NodeID: "server1"},
	}
	if err := r.restoreAttachmentTickets(pvc.Spec.VolumeName, "fallback", tickets, &longhornv1beta2.VolumeAttachment{}); err != nil {
		t.Fatal(err)
	}
	if gotID != "csi-test" || gotType != "csi-attacher" || gotNode != "server1" {
		t.Fatalf("unexpected restored ticket id=%q type=%q node=%q", gotID, gotType, gotNode)
	}
}

func TestPVCResizeParsesLegacyPodNames(t *testing.T) {
	pods, err := resizePodSnapshots(`["mysql-0"]`)
	if err != nil {
		t.Fatal(err)
	}
	if len(pods) != 1 || pods[0].Name != "mysql-0" || !pods[0].WaitForRestart {
		t.Fatalf("unexpected legacy pod snapshots %#v", pods)
	}
}
