package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChildDisplayEndpointsUseMainSettings(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("CKM_SYNC_TOKEN", "internal-secret")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/main-settings" || r.Header.Get("X-W7Panel-CKM-Sync-Token") != "internal-secret" {
			t.Errorf("unexpected main settings request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"siteSetting":{"spec":{"login":{"indexPage":"resource"},"general":{"filing":{"icp":"主站备案"},"contactConfigs":[{"name":"主站联系","type":"text","text":"主站电话","showName":true}]}}},"configMaps":{},"domainParse":{"spec":{"type":"A","ips":["1.2.3.4"]}}}}`))
	}))
	defer upstream.Close()
	t.Setenv("CKM_SYNC_ENDPOINT", upstream.URL)
	for _, test := range []struct {
		name    string
		handler func(*gin.Context)
		want    string
	}{
		{"filing", Site{}.Beian, "主站备案"},
		{"contacts", Site{}.Lianxi, "主站电话"},
		{"home", Site{}.K3kConfig, "resource"},
		{"domain", DomainParse{}.Get, "1.2.3.4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			test.handler(ctx)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.want) {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
	t.Setenv("CKM_SYNC_ENDPOINT", "")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	Site{}.Lianxi(ctx)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("missing main sync must not use local contacts: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
