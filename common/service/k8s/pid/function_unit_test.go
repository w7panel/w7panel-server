package pid

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestResolveContainerStatusRequiresTargetForMultipleContainers(t *testing.T) {
	pod := podWithStatuses(
		containerStatus("app", "containerd://app-id"),
		containerStatus("sidecar", "containerd://sidecar-id"),
	)

	_, err := resolveContainerStatus(pod, "", "")
	if err == nil {
		t.Fatal("expected multiple container pod to require containerName or containerId")
	}
}

func TestResolveContainerStatusByName(t *testing.T) {
	pod := podWithStatuses(
		containerStatus("app", "containerd://app-id"),
		containerStatus("sidecar", "containerd://sidecar-id"),
	)

	status, err := resolveContainerStatus(pod, "sidecar", "")
	if err != nil {
		t.Fatal(err)
	}
	if status.Name != "sidecar" {
		t.Fatalf("expected sidecar, got %s", status.Name)
	}
}

func TestResolveContainerStatusByNormalizedContainerID(t *testing.T) {
	pod := podWithStatuses(
		containerStatus("app", "containerd://app-id"),
		containerStatus("sidecar", "containerd://sidecar-id"),
	)

	status, err := resolveContainerStatus(pod, "", "sidecar-id")
	if err != nil {
		t.Fatal(err)
	}
	if status.Name != "sidecar" {
		t.Fatalf("expected sidecar, got %s", status.Name)
	}
}

func TestAnnotationContainerPidByContainerName(t *testing.T) {
	pod := podWithStatuses(
		containerStatus("app", "containerd://app-id"),
		containerStatus("sidecar", "containerd://sidecar-id"),
	)
	pod.Annotations = map[string]string{
		legacyPidAnnotation: "100",
		legacyCIDAnnotation: "containerd://old-id",
	}

	if err := setAnnotationContainerPid(pod, "sidecar", "containerd://sidecar-id", 200); err != nil {
		t.Fatal(err)
	}
	pid, err := getAnnotationPodPid(pod, "sidecar", "sidecar-id")
	if err != nil {
		t.Fatal(err)
	}
	if pid != 200 {
		t.Fatalf("expected pid 200, got %d", pid)
	}
	if _, ok := pod.Annotations[legacyPidAnnotation]; ok {
		t.Fatal("expected legacy pid annotation to be removed")
	}
	if _, ok := pod.Annotations[legacyCIDAnnotation]; ok {
		t.Fatal("expected legacy container-id annotation to be removed")
	}
}

func TestAnnotationContainerPidRejectsStaleContainerID(t *testing.T) {
	pod := podWithStatuses(containerStatus("app", "containerd://new-id"))
	if err := setAnnotationContainerPid(pod, "app", "containerd://old-id", 100); err != nil {
		t.Fatal(err)
	}

	_, err := getAnnotationPodPid(pod, "app", "containerd://new-id")
	if err == nil {
		t.Fatal("expected stale container id to be rejected")
	}
}

func podWithStatuses(statuses ...corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: statuses,
		},
	}
}

func containerStatus(name, containerID string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:        name,
		ContainerID: containerID,
		State: corev1.ContainerState{
			Running: &corev1.ContainerStateRunning{},
		},
	}
}
