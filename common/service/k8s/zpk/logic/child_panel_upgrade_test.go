package logic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
)

func TestCKMChildPanelUpgradeUsesExistingInstallRoute(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("SVC_NAME", "w7panel-k3k-agent-demo")
	pack := &types.ManifestPackage{}
	pack.Manifest.Application.Identifie = "w7panel-offline"
	params := InstallRequest{Namespace: "default", ReleaseName: "w7panel-k3k-agent-demo", RepoUrl: "https://zpk.w7.cc/zpk/respo/info/w7panel_offline"}
	if !isCKMChildPanelUpgrade(params, pack) {
		t.Fatal("child panel must use HelmChart upgrade even without AppGroup")
	}
	params.ReleaseName = "another-release"
	if isCKMChildPanelUpgrade(params, pack) {
		t.Fatal("other releases must use normal installer")
	}
}

func TestRequestCKMChildPanelUpgrade(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync/child-panel-chart" || r.Header.Get("X-W7Panel-CKM-Sync-Token") != "test-token" {
			t.Errorf("bad request: %s", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["releaseName"] != "panel" || payload["version"] != "1.1.110" {
			t.Errorf("payload = %v", payload)
		}
	}))
	defer server.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", server.URL+"/sync")
	t.Setenv("CKM_SYNC_TOKEN", "test-token")
	t.Setenv("CKM_NAME", "demo")
	t.Setenv("K3K_NAME", "owner-demo")
	t.Setenv("K3K_NAMESPACE", "k3k-owner")
	if err := requestCKMChildPanelUpgrade(context.Background(), "panel", "1.1.110"); err != nil {
		t.Fatal(err)
	}
}
