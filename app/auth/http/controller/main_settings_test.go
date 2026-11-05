package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/panelauth"
)

func TestMainSettingsRejectsHostAndMissingPrincipal(t *testing.T) {
	for _, principal := range []any{nil, panelauth.Principal{TokenUse: panelauth.TokenUsePanel}} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
		if principal != nil {
			ctx.Set("panel_principal", principal)
		}
		MainSettings{}.Get(ctx)
		if ctx.Writer.Status() != http.StatusForbidden {
			t.Fatalf("principal %v received status %d", principal, ctx.Writer.Status())
		}
	}
}

func TestChildMainSettingsUsesSyncCredentialAndFiltersResponse(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_TOKEN", "internal-secret")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ckm-api/v1/internal/sync/main-settings" || r.Header.Get("X-W7Panel-CKM-Sync-Token") != "internal-secret" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected upstream request: %s %#v", r.URL.Path, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"data":{"siteSetting":null,"configMaps":{},"domainParse":null,"private":"must-not-leak"}}`))
	}))
	defer upstream.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", upstream.URL+"/ckm-api/v1/internal/sync")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
	ctx.Request.Header.Set("Authorization", "Bearer local-panel-user")
	ctx.Set("panel_principal", panelauth.Principal{TokenUse: panelauth.TokenUsePanel})
	MainSettings{}.Get(ctx)
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "must-not-leak") || !strings.Contains(recorder.Body.String(), `"siteSetting":null`) {
		t.Fatalf("unexpected response %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestChildMainSettingsRejectsOtherSessionsAndMissingSyncConfig(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	for _, principal := range []panelauth.Principal{{TokenUse: panelauth.TokenUseExternalAPI}} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
		ctx.Set("panel_principal", principal)
		MainSettings{}.Get(ctx)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("token use %q received %d", principal.TokenUse, recorder.Code)
		}
	}
	t.Setenv("CKM_SYNC_ENDPOINT", "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
	ctx.Set("panel_principal", panelauth.Principal{TokenUse: panelauth.TokenUsePanel})
	MainSettings{}.Get(ctx)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing endpoint received %d", recorder.Code)
	}
}

func TestChildMainSettingsDoesNotForwardSyncTokenOnRedirect(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_TOKEN", "internal-secret")
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected = true
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer upstream.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", upstream.URL)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
	ctx.Set("panel_principal", panelauth.Principal{TokenUse: panelauth.TokenUsePanel})
	MainSettings{}.Get(ctx)
	if recorder.Code != http.StatusBadGateway || redirected {
		t.Fatalf("redirect response=%d, followed=%t", recorder.Code, redirected)
	}
}
