package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"k8s.io/client-go/rest"
)

func TestChildLicenseRegistersForOwnerWithStableSiteID(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("K3K_NAME", "tenant-a")
	t.Setenv("CKM_NAME", "cluster-a")
	oldAPI, oldLicense := consoleApi, GetCurrentLicense()
	t.Cleanup(func() {
		SetConsoleApi(oldAPI)
		SetCurrentLicense(oldLicense)
	})

	var siteIDs []string
	var siteURLs []string
	var savedLicense map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/w7panel.w7.com/v1alpha1/users":
			fmt.Fprint(w, `{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"UserList","items":[{"metadata":{"name":"another-user"},"spec":{"license":{"appId":"other","appSecret":"other","founderSaName":"another-user"}}},{"metadata":{"name":"tenant-a"},"spec":{"cloudOpenid":"owner-openid"}}]}`)
		case "/apis/w7panel.w7.com/v1alpha1/users/tenant-a":
			if r.Method == http.MethodPut {
				var obj struct {
					Spec map[string]any `json:"spec"`
				}
				if err := json.NewDecoder(r.Body).Decode(&obj); err != nil {
					t.Errorf("decode user update: %v", err)
				}
				savedLicense, _ = obj.Spec["license"].(map[string]any)
			}
			fmt.Fprint(w, `{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"User","metadata":{"name":"tenant-a"},"spec":{"cloudOpenid":"owner-openid"}}`)
		case "/api/thirdparty-cd/k8s-offline/openid-to-cd-token":
			if got := r.FormValue("openid"); got != "owner-openid" {
				t.Errorf("OpenID = %q", got)
			}
			fmt.Fprint(w, `{"token":"owner-token"}`)
		case "/api/thirdparty-cd/k8s-offline/license/register":
			if got := r.Header.Get("Authorization"); got != "Bearer owner-token" {
				t.Errorf("authorization = %q", got)
			}
			siteIDs = append(siteIDs, r.FormValue("sn"))
			siteURLs = append(siteURLs, r.FormValue("offlineUrl"))
			fmt.Fprint(w, `{"appId":"child-app","appSecret":"child-secret"}`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	SetConsoleApi(server.URL)
	sdk, err := k8s.NewForRestConfig(&rest.Config{Host: server.URL}, "default")
	if err != nil {
		t.Fatal(err)
	}
	client := NewLicenseClient(nil, sdk)
	for _, ignoreExisting := range []bool{false, true} {
		if _, err := client.CreateLicenseSite("tenant-a", ignoreExisting); err != nil {
			t.Fatal(err)
		}
	}
	if len(siteIDs) != 2 || siteIDs[0] == "" || siteIDs[0] != siteIDs[1] {
		t.Fatalf("site IDs = %v, want two equal nonempty values", siteIDs)
	}
	if len(siteURLs) != 2 || siteURLs[0] != siteURLs[1] || !strings.Contains(siteURLs[0], siteIDs[0]) {
		t.Fatalf("site URLs = %v, want stable URL containing the child site ID", siteURLs)
	}
	if savedLicense["appId"] != "child-app" || savedLicense["founderSaName"] != "tenant-a" {
		t.Fatalf("saved license = %v", savedLicense)
	}
	if _, err := client.CreateLicenseSite("another-user", false); err == nil {
		t.Fatal("another user was allowed to register the child license")
	}
}
