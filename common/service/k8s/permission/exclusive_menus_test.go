package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestExclusiveMenusValidate(t *testing.T) {
	valid := ExclusiveMenus{Rules: []ExclusiveMenuRule{
		{MenuRule: "system", ClusterTypes: []string{"main"}},
		{MenuRule: "system/audit", ClusterTypes: []string{"main"}},
	}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, rules := range [][]ExclusiveMenuRule{
		{{MenuRule: "system", ClusterTypes: []string{"main", "main"}}},
		{{MenuRule: "system/*", ClusterTypes: []string{"main"}}},
		{{MenuRule: "system", ClusterTypes: []string{"main"}}, {MenuRule: "system/audit", ClusterTypes: []string{"child"}}},
	} {
		if err := (ExclusiveMenus{Rules: rules}).Validate(); err == nil {
			t.Fatalf("accepted invalid rules: %+v", rules)
		}
	}
}

func TestSyncExclusiveMenusKeepsCacheOnFailure(t *testing.T) {
	policy := ExclusiveMenus{Rules: []ExclusiveMenuRule{{MenuRule: "system", ClusterTypes: []string{"main"}}}, ResourceVersion: "remote"}
	response := policy
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exclusive-menus" || r.Header.Get("X-W7Panel-CKM-Sync-Token") != "test" {
			t.Errorf("unexpected sync request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer remote.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", remote.URL)
	t.Setenv("CKM_SYNC_TOKEN", "test")
	var cached *corev1.ConfigMap
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if cached == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","reason":"NotFound","code":404}`))
				return
			}
		case http.MethodPost, http.MethodPut:
			var next corev1.ConfigMap
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
				t.Error(err)
			}
			next.ResourceVersion = "1"
			cached = &next
		}
		_ = json.NewEncoder(w).Encode(cached)
	}))
	defer local.Close()
	kubeconfig := filepath.Join(t.TempDir(), "config")
	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters:\n- name: test\n  cluster:\n    server: %s\nusers:\n- name: test\n  user:\n    token: test\ncontexts:\n- name: test\n  context:\n    cluster: test\n    user: test\ncurrent-context: test\n", local.URL)
	if err := os.WriteFile(kubeconfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	sdk := k8s.NewK8sClient().Sdk
	client, err := kubernetes.NewForConfig(&rest.Config{Host: local.URL, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
	if err != nil {
		t.Fatal(err)
	}
	sdk.ClientSet = client
	if err := SyncExclusiveMenus(context.Background(), sdk); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadExclusiveMenus(context.Background(), sdk)
	if err != nil || len(loaded.Rules) != 1 {
		t.Fatalf("cache = %+v, %v", loaded, err)
	}
	response = ExclusiveMenus{Rules: []ExclusiveMenuRule{}}
	if err := SyncExclusiveMenus(context.Background(), sdk); err != nil {
		t.Fatal(err)
	}
	loaded, err = LoadExclusiveMenus(context.Background(), sdk)
	if err != nil || len(loaded.Rules) != 0 {
		t.Fatalf("empty policy = %+v, %v", loaded, err)
	}
	response = ExclusiveMenus{}
	if err := SyncExclusiveMenus(context.Background(), sdk); err == nil {
		t.Fatal("invalid response accepted")
	}
	loaded, err = LoadExclusiveMenus(context.Background(), sdk)
	if err != nil || len(loaded.Rules) != 0 {
		t.Fatalf("failure changed cache: %+v, %v", loaded, err)
	}
}
