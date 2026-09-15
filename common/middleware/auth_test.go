package middleware

import "testing"

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
