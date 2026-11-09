package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChildDomainWhiteListUsesLocalUserWithoutSync(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_ENDPOINT", "")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/apis/w7panel.w7.com/v1alpha1/users/") {
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/child-user") {
			_, _ = w.Write([]byte(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"User","metadata":{"name":"child-user"},"spec":{"domainWhiteList":[{"domain":"child.example.com"}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"User","metadata":{"name":"empty-user"},"spec":{}}`))
		}
	}))
	defer upstream.Close()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\nusers:\n- name: test\n  user:\n    token: test\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\n", upstream.URL)
	if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	for _, test := range []struct{ username, want string }{{"child-user", "child.example.com"}, {"empty-user", `"data":[]`}} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/domain-white-list", nil)
		ctx.Set("username", test.username)
		Auth{}.DomainWhiteList(ctx)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.want) {
			t.Fatalf("user %s: unexpected response %d: %s", test.username, recorder.Code, recorder.Body.String())
		}
	}
}
