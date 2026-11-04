package pluginproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlerRewritesLegacyPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/traffic/apps" || r.URL.Query().Get("page") != "2" || r.Header.Get("X-W7Panel-Token") != "signed" {
			t.Errorf("unexpected proxy request: %s %s", r.URL, r.Header.Get("X-W7Panel-Token"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	t.Setenv("METRICS_TRAFFIC_URL", upstream.URL)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/panel-api/v1/traffic/*path", Handler("METRICS_TRAFFIC_URL", "", "/panel-api/v1/traffic", "/api/v1/traffic"))
	panel := httptest.NewServer(router)
	defer panel.Close()
	r, err := http.NewRequest(http.MethodGet, panel.URL+"/panel-api/v1/traffic/apps?page=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-W7Panel-Token", "signed")
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
