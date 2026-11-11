package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
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

func TestSyncChildDomainWhiteListKeepsLocalDataOnFailure(t *testing.T) {
	remoteReplies := make(chan struct {
		status int
		body   string
	}, 4)
	remoteReplies <- struct {
		status int
		body   string
	}{http.StatusOK, `{"domainWhiteList":[{"domain":"new.example.com"}]}`}
	remoteReplies <- struct {
		status int
		body   string
	}{http.StatusOK, `{"domainWhiteList":[]}`}
	remoteReplies <- struct {
		status int
		body   string
	}{http.StatusBadGateway, ""}
	remoteReplies <- struct {
		status int
		body   string
	}{http.StatusOK, `{"domainWhiteList":[{"domain":"new.example.com"}]}`}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/domain-white-list" || r.Header.Get("X-W7Panel-CKM-Sync-Token") != "sync-token" {
			t.Errorf("unexpected sync request: %s", r.URL.Path)
		}
		reply := <-remoteReplies
		w.WriteHeader(reply.status)
		_, _ = w.Write([]byte(reply.body))
	}))
	defer remote.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", remote.URL)
	t.Setenv("CKM_SYNC_TOKEN", "sync-token")

	patches := make(chan []string, 2)
	patchStatuses := make(chan int, 3)
	patchStatuses <- http.StatusOK
	patchStatuses <- http.StatusOK
	patchStatuses <- http.StatusInternalServerError
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/apis/w7panel.w7.com/v1alpha1/users/child-user" {
			t.Errorf("unexpected local request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil || len(patch) != 1 {
			t.Errorf("invalid patch: %v, %v", patch, err)
		}
		var spec map[string][]struct {
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(patch["spec"], &spec); err != nil || len(spec) != 1 {
			t.Errorf("patch changed other user fields: %v, %v", spec, err)
		}
		var domains []string
		for _, item := range spec["domainWhiteList"] {
			domains = append(domains, item.Domain)
		}
		patches <- domains
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(<-patchStatuses)
		_, _ = w.Write([]byte(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"User","metadata":{"name":"child-user"},"spec":{}}`))
	}))
	defer local.Close()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\nusers:\n- name: test\n  user:\n    token: test\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\n", local.URL)
	if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	sdk := k8s.NewK8sClient().Sdk
	if err := syncChildDomainWhiteList(context.Background(), sdk, "child-user"); err != nil {
		t.Fatal(err)
	}
	if domains := <-patches; len(domains) != 1 || domains[0] != "new.example.com" {
		t.Fatalf("successful sync list = %v", domains)
	}
	if err := syncChildDomainWhiteList(context.Background(), sdk, "child-user"); err != nil {
		t.Fatal(err)
	}
	if domains := <-patches; len(domains) != 0 {
		t.Fatalf("empty sync list = %v", domains)
	}
	if err := syncChildDomainWhiteList(context.Background(), sdk, "child-user"); err == nil || len(patches) != 0 {
		t.Fatalf("failed sync changed local list: pending patches=%d err=%v", len(patches), err)
	}
	if err := syncChildDomainWhiteList(context.Background(), sdk, "child-user"); err == nil {
		t.Fatal("failed local patch must be reported to the background logger")
	}
}
