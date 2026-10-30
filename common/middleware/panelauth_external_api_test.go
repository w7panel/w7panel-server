package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/panelauth"
)

func TestExternalAPIUsesServiceAccountWithoutUser(t *testing.T) {
	var tokenRequested bool
	kubeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/w7panel.w7.com/v1alpha1/users/api":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
		case "/apis/w7panel.w7.com/v1alpha1/permissions/api":
			fmt.Fprint(w, `{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"Permission","metadata":{"name":"api"},"spec":{"role":"api","apiRules":[{"path":"/panel-api/v1/namespaces","method":["get"]}]}}`)
		case "/api/v1/namespaces/default/configmaps/w7panel-permission-exclusives":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`)
		case "/api/v1/namespaces/default/serviceaccounts/api/token":
			if r.Method != http.MethodPost {
				t.Errorf("token request method = %s", r.Method)
			}
			tokenRequested = true
			fmt.Fprint(w, `{"apiVersion":"authentication.k8s.io/v1","kind":"TokenRequest","status":{"token":"short-lived-api-token"}}`)
		default:
			t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer kubeAPI.Close()

	kubeconfig := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\n    namespace: default\ncurrent-context: test\nusers:\n- name: test\n  user:\n    token: test\n", kubeAPI.URL)
	if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/namespaces", nil)
	processPanelPrincipal(ctx, &panelauth.Principal{Username: "api", PermissionName: "api", Role: "api", TokenUse: panelauth.TokenUseExternalAPI})
	if w.Code != http.StatusOK || !tokenRequested {
		t.Fatalf("status = %d, token requested = %v, body = %s", w.Code, tokenRequested, w.Body.String())
	}
	if token, ok := K8sTokenFromContext(ctx.Request.Context()); !ok || token != "short-lived-api-token" {
		t.Fatal("external API request did not receive the api ServiceAccount token")
	}
}
