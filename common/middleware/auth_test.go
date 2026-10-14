package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUsesPanelAuth(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{path: "/panel-api/v1/microapp/top", want: true},
		{path: "/k8s-proxy/v1/namespaces/default/pods", want: true},
		{path: "/k8s-proxy", want: false},
		{path: "/healthz", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := usesPanelAuth(tt.path); got != tt.want {
				t.Fatalf("usesPanelAuth(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestK8sProxyReceivesServerMintedCredential(t *testing.T) {
	if !requiresLegacyK8sCredential("/k8s-proxy/v1/namespaces/default/pods") {
		t.Fatal("k8s proxy must receive the server-minted Kubernetes credential")
	}
}

func TestRegistryWriteClassification(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: "GET", path: "/v2/demo/manifests/latest", want: false},
		{method: "HEAD", path: "/v2/demo/blobs/sha256:abc", want: false},
		{method: "POST", path: "/v2/demo/blobs/uploads/", want: true},
		{method: "PUT", path: "/v2/demo/manifests/latest", want: true},
		{method: "PATCH", path: "/v2/demo/blobs/uploads/id", want: true},
		{method: "DELETE", path: "/v2/demo/manifests/sha256:abc", want: true},
		{method: "PUT", path: "/panel-api/v1/registry/patch/images/tag", want: false},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		if got := IsRegistryWriteRequest(req); got != tt.want {
			t.Fatalf("IsRegistryWriteRequest(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestRegistryWriteUsesConfiguredAuthMode(t *testing.T) {
	req := httptest.NewRequest("PUT", "/v2/demo/manifests/latest", nil)
	t.Setenv("W7PANEL_AUTH_MODE", PanelAuthMode)
	if !shouldUsePanelAuth(req) {
		t.Fatal("registry write must use panel auth in panel mode")
	}
	t.Setenv("W7PANEL_AUTH_MODE", K8sAuthMode)
	if shouldUsePanelAuth(req) {
		t.Fatal("registry write must use Kubernetes auth in k8s mode")
	}
}

func TestRegistryServiceAccountToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/v2/demo/manifests/latest", nil)
	req.SetBasicAuth(registryServiceAccountUsername, "service-account-jwt")
	token, ok := registryServiceAccountToken(req)
	if !ok || token != "service-account-jwt" {
		t.Fatalf("registryServiceAccountToken() = (%q, %v)", token, ok)
	}

	req.SetBasicAuth("admin", "w7-secret")
	if _, ok := registryServiceAccountToken(req); ok {
		t.Fatal("ordinary Docker Basic credentials must not authenticate registry writes")
	}
}
