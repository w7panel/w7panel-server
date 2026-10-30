package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/w7panel/w7panel/common/service/panelauth"
)

func TestChildMainSettingsReadExceptionIsPanelGetOnly(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
	panel := &panelauth.Principal{TokenUse: panelauth.TokenUsePanel}
	t.Setenv("IS_CHILD", "true")
	if !childMainSettingsReadAllowed(panel, request) {
		t.Fatal("child panel read should be allowed")
	}
	if requiresLegacyK8sCredential(request.URL.Path) {
		t.Fatal("display-only settings must not mint a Kubernetes credential")
	}
	for _, use := range []string{panelauth.TokenUseExternalAPI, panelauth.TokenUseCKMPanel} {
		if childMainSettingsReadAllowed(&panelauth.Principal{TokenUse: use}, request) {
			t.Fatalf("token use %q bypassed route permission", use)
		}
	}
	if childMainSettingsReadAllowed(panel, httptest.NewRequest(http.MethodPost, request.URL.Path, nil)) || childMainSettingsReadAllowed(panel, httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/other", nil)) {
		t.Fatal("other methods or paths bypassed route permission")
	}
	t.Setenv("IS_CHILD", "false")
	if childMainSettingsReadAllowed(panel, request) {
		t.Fatal("host panel bypassed route permission")
	}
}
