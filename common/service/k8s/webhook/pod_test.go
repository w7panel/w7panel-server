package webhook

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLxcfsAnnotationEnabled(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Annotations: map[string]string{"w7.cc/lxcfs": "true"},
	}}

	if !isLxcfsAnnotationEnabled(pod) {
		t.Fatal("expected lxcfs annotation to be enabled")
	}
}

func TestInjectLxcfs(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	if !injectLxcfs(pod) {
		t.Fatal("expected lxcfs injection to modify pod")
	}
	if len(pod.Spec.Volumes) != len(volumesTemplate) {
		t.Fatalf("volumes = %d, want %d", len(pod.Spec.Volumes), len(volumesTemplate))
	}
	if len(pod.Spec.Containers[0].VolumeMounts) != len(volumeMountsTemplate) {
		t.Fatalf("volume mounts = %d, want %d", len(pod.Spec.Containers[0].VolumeMounts), len(volumeMountsTemplate))
	}

	if injectLxcfs(pod) {
		t.Fatal("expected repeated lxcfs injection to be a no-op")
	}
	if len(pod.Spec.Volumes) != len(volumesTemplate) {
		t.Fatalf("volumes after repeated injection = %d, want %d", len(pod.Spec.Volumes), len(volumesTemplate))
	}
	if len(pod.Spec.Containers[0].VolumeMounts) != len(volumeMountsTemplate) {
		t.Fatalf("volume mounts after repeated injection = %d, want %d", len(pod.Spec.Containers[0].VolumeMounts), len(volumeMountsTemplate))
	}
}
