package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/panelauth"
)

func TestCKMWebSocketDoesNotFallBackToHostCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/panel-api/v1/exec", nil)
	req.AddCookie(&http.Cookie{Name: panelSessionCookie, Value: "host"})
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Protocol", "w7panel-ckm, w7panel-bearer.child-session")
	if got := panelToken(req); got != "child-session" {
		t.Fatalf("got %q", got)
	}
}

func TestCKMPanelCannotMintSessionOrSelectHostCluster(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test")
	p := panelauth.Principal{Username: "alice", Actor: "admin", CKMUID: "a-uid", CVMName: "a", K3KNamespace: "k3k-alice", Role: "normal", PermissionName: "normal", TokenUse: panelauth.TokenUseCKMPanel}
	raw, err := panelauth.Issue(p, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/panel-api/v1/auth/ckm-session", "/panel-api/v1/auth/k8s-credentials/token", "/panel-api/v1/oidc/js-code", "/k8s-proxy/api/v1/namespaces?local=1", "/k8s-proxy/api/v1/namespaces?local=true"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", path, nil)
			c.Request.Header.Set("Authorization", "Bearer "+raw)
			c.Request.Header.Set("X-W7Panel-K8s-Token", "another-target")
			if c.Request.URL.Path[:10] == "/k8s-proxy" {
				K8sAuth{}.Process(c)
			} else {
				processCKMPanel(c, p)
			}
			if w.Code != http.StatusForbidden {
				t.Fatalf("got %d", w.Code)
			}
		})
	}
}
