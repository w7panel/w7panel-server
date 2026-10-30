package permission

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestChildReadsMainPolicyAndLatestWhiteList(t *testing.T) {
	var response atomic.Value
	response.Store(`{"policy":{"rules":[{"menuRule":"system/audit","clusterTypes":["main"]}]}}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-W7Panel-CKM-Sync-Token") != "sync-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/ckm-api/v1/internal/sync/domain-white-list" {
			_, _ = w.Write([]byte(`{"domainWhiteList":[{"domain":"second.example.com"}]}`))
			return
		}
		if r.URL.Path != "/ckm-api/v1/internal/sync/panel-policy" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(response.Load().(string)))
	}))
	defer upstream.Close()
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_ENDPOINT", upstream.URL+"/ckm-api/v1/internal/sync")
	t.Setenv("CKM_SYNC_TOKEN", "sync-token")
	policy, err := LoadExclusivePolicy(context.Background(), nil)
	if err != nil || policy.AllowsMenu(ChildClusterType, "system/audit") || !policy.AllowsMenu(MainClusterType, "system/audit") {
		t.Fatalf("child policy = %#v, err = %v", policy, err)
	}
	whiteList, err := LoadMainDomainWhiteList(context.Background())
	if err != nil || len(whiteList) != 1 || whiteList[0].Domain != "second.example.com" {
		t.Fatalf("latest whitelist = %#v, err = %v", whiteList, err)
	}
	response.Store(`{"policy":{"rules":[{"menuRule":"system/audit","exclusiveGroups":["ops"]}]}}`)
	if _, err := LoadExclusivePolicy(context.Background(), nil); err == nil {
		t.Fatal("legacy group policy was accepted")
	}
}
