package longhorn

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDefaultDiskInstallLifecycle(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"}}}
	if !markDefaultDiskInstall(node) {
		t.Fatal("control-plane node was not marked for first install")
	}
	if node.Labels[defaultDiskLabel] != "config" || node.Annotations[defaultDiskAnnotation] != defaultDiskConfig {
		t.Fatal("Longhorn default disk configuration was not written")
	}
	if clearDefaultDiskInstall(node, false, false) {
		t.Fatal("pending install was cleared before the disk appeared")
	}
	if !clearDefaultDiskInstall(node, true, false) {
		t.Fatal("completed install configuration was not cleared")
	}
	if node.Labels[defaultDiskLabel] != "" || node.Annotations[defaultDiskAnnotation] != "" || node.Annotations[defaultDiskPending] != "" {
		t.Fatal("default disk trigger remained after installation")
	}
}

func TestDefaultDiskLegacyCleanup(t *testing.T) {
	legacy := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{defaultDiskLabel: "config"},
		Annotations: map[string]string{defaultDiskAnnotation: defaultDiskConfig},
	}}
	if !clearDefaultDiskInstall(legacy, false, false) {
		t.Fatal("legacy trigger was not cleared after disk deletion")
	}
	formatted := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels: map[string]string{defaultDiskLabel: "config"},
		Annotations: map[string]string{defaultDiskAnnotation: `[
  {
    "path": "/var/lib/longhorn",
    "allowScheduling": true,
    "name": "disk-default",
    "tags": ["disk-default"]
  }
]`},
	}}
	if !clearDefaultDiskInstall(formatted, false, false) {
		t.Fatal("formatted legacy trigger was not cleared")
	}
	custom := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{defaultDiskLabel: "config"},
		Annotations: map[string]string{defaultDiskAnnotation: `[{"name":"custom"}]`},
	}}
	if clearDefaultDiskInstall(custom, false, false) {
		t.Fatal("custom Longhorn disk configuration was changed")
	}
	extraField := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Labels:      map[string]string{defaultDiskLabel: "config"},
		Annotations: map[string]string{defaultDiskAnnotation: `[{"path":"/var/lib/longhorn","allowScheduling":true,"name":"disk-default","tags":["disk-default"],"storageReserved":1}]`},
	}}
	if clearDefaultDiskInstall(extraField, false, false) {
		t.Fatal("custom Longhorn disk field was changed")
	}
	worker := &corev1.Node{}
	if markDefaultDiskInstall(worker) {
		t.Fatal("new worker node was marked for a default disk")
	}
	failedInstall := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true"}}}
	markDefaultDiskInstall(failedInstall)
	if !clearDefaultDiskInstall(failedInstall, false, true) {
		t.Fatal("failed installation left a default disk trigger")
	}
}
