package permission

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChildReadsMainDomainWhiteList(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-W7Panel-CKM-Sync-Token") != "sync-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/ckm-api/v1/internal/sync/domain-white-list" {
			_, _ = w.Write([]byte(`{"domainWhiteList":[{"domain":"second.example.com"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_ENDPOINT", upstream.URL+"/ckm-api/v1/internal/sync")
	t.Setenv("CKM_SYNC_TOKEN", "sync-token")
	whiteList, err := LoadMainDomainWhiteList(context.Background())
	if err != nil || len(whiteList) != 1 || whiteList[0].Domain != "second.example.com" {
		t.Fatalf("latest whitelist = %#v, err = %v", whiteList, err)
	}
}
