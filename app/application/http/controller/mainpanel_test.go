package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	microappv1 "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalMicroAppsUsesRequestAddress(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://panel.example.com/panel-api/v1/noauth/microapp/normal", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	items := normalMicroApps(request, &microappv1.MicroAppList{Items: []microappv1.MicroApp{{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-root"},
		Spec:       microappv1.MicroAppSpec{Title: "Demo"},
	}}})
	if len(items) != 1 || items[0].Title != "Demo" || items[0].URL != "https://panel.example.com/appgroup/demo-root/micro" {
		t.Fatalf("normal microapps = %#v", items)
	}
}

func TestAppInfoReturnsMainPanelURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("MAIN_PANEL_URL", "https://panel.example.com")
	facade.Config = viper.New()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	Helm{}.AppInfo(ctx)
	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data["mainPanelUrl"] != "https://panel.example.com" {
		t.Fatalf("mainPanelUrl = %#v", response.Data["mainPanelUrl"])
	}
}

func TestNormalMicroAppsResponseSupportsJSONP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/?callback=apps", nil)
	MicroApp{}.normalMicroAppsResponse(ctx, []normalMicroApp{{Title: "Demo"}})
	if got, want := recorder.Body.String(), "apps([{\"title\":\"Demo\",\"url\":\"\"}]);"; got != want {
		t.Fatalf("JSONP response = %q, want %q", got, want)
	}
}
