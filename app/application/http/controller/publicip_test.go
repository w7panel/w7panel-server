package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPublicSANs(t *testing.T) {
	nodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true", publicIPLabel: "203.0.113.1"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/master": "true", publicIPLabel: "203.0.113.2"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"node-role.kubernetes.io/control-plane": "true", publicIPLabel: "203.0.113.1"}}},
		{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{publicIPLabel: "203.0.113.3"}}},
	}
	got := publicSANs(nodes)
	if len(got) != 2 || got[0] != "203.0.113.1" || got[1] != "203.0.113.2" {
		t.Fatalf("public SANs = %v", got)
	}
}

func TestFilterPublicSANs(t *testing.T) {
	got := filterPublicSANs("old.example.com,203.0.113.1,custom.example.com", "old.example.com", []string{"203.0.113.1"})
	if got != "custom.example.com" {
		t.Fatalf("custom SANs = %q", got)
	}
}
