package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPublicIPHTTPReachable(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusForbidden)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/livez" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		w.WriteHeader(int(status.Load()))
	}))
	if !publicIPHTTPReachable(context.Background(), server.URL) {
		t.Fatal("self-signed TLS endpoint returning HTTP 403 should be reachable")
	}
	status.Store(http.StatusInternalServerError)
	if publicIPHTTPReachable(context.Background(), server.URL) {
		t.Fatal("unhealthy endpoint should not be accepted")
	}
	server.Close()
	if publicIPHTTPReachable(context.Background(), server.URL) {
		t.Fatal("closed endpoint should be unreachable")
	}
}

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
