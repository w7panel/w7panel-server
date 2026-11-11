package permission

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMainDomainWhiteListUsesCurrentPermission(t *testing.T) {
	t.Setenv("IS_CHILD", "false")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/apis/w7panel.w7.com/v1alpha1/permissions/custom" {
			t.Errorf("unexpected permission request: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"Permission","metadata":{"name":"custom"},"spec":{"domainWhiteList":[{"domain":"custom.example.com"}]}}`))
	}))
	defer upstream.Close()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\nusers:\n- name: test\n  user:\n    token: test\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\n", upstream.URL)
	if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	whiteList, err := LoadMainDomainWhiteList(context.Background(), "custom")
	if err != nil || len(whiteList) != 1 || whiteList[0].Domain != "custom.example.com" {
		t.Fatalf("current permission whitelist = %#v, err = %v", whiteList, err)
	}
}

func TestMainDomainWhiteListRequiresCurrentPermission(t *testing.T) {
	t.Setenv("IS_CHILD", "false")
	if _, err := LoadMainDomainWhiteList(context.Background(), ""); err == nil {
		t.Fatal("missing current permission must not fall back to normal")
	}
}
