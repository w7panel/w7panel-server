package middleware

import (
	"net/http/httptest"
	"testing"
)

func TestCkmAuthMode(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "default", want: K8sAuthMode},
		{name: "panel", env: PanelAuthMode, want: PanelAuthMode},
		{name: "k8s", env: K8sAuthMode, want: K8sAuthMode},
		{name: "invalid", env: "unknown", want: K8sAuthMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("W7PANEL_AUTH_MODE", tt.env)
			if got := ckmAuthMode(); got != tt.want {
				t.Fatalf("ckmAuthMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPanelTokenHeaderMode(t *testing.T) {
	t.Setenv("W7PANEL_AUTH_MODE", PanelAuthMode)
	req := httptest.NewRequest("GET", "/panel-api/v1/test", nil)
	req.Header.Set("X-W7Panel-Token", "panel-token")
	req.Header.Set("Authorization", "Bearer authorization-token")
	if got := panelToken(req); got != "panel-token" {
		t.Fatalf("panelToken() = %q, want panel-token", got)
	}

	t.Setenv("W7PANEL_AUTH_MODE", K8sAuthMode)
	if got := panelToken(req); got != "authorization-token" {
		t.Fatalf("panelToken() = %q, want authorization-token", got)
	}
}

func TestPanelTokenRejectsAPITokenQuery(t *testing.T) {
	download := httptest.NewRequest("GET", "/panel-api/v1/download/upload/source.zip?api-token=download-token", nil)
	if got := panelToken(download); got != "" {
		t.Fatalf("panelToken(download) = %q, want empty", got)
	}

	nonDownload := httptest.NewRequest("GET", "/panel-api/v1/namespaces?api-token=download-token", nil)
	if got := panelToken(nonDownload); got != "" {
		t.Fatalf("panelToken(non-download) = %q, want empty", got)
	}

	download.Header.Set("Authorization", "Bearer authorization-token")
	if got := panelToken(download); got != "authorization-token" {
		t.Fatalf("panelToken(download with Authorization) = %q, want authorization-token", got)
	}
}
