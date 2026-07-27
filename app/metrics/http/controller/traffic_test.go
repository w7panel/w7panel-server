package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

func initTrafficTestConfig() {
	facade.Config = viper.New()
	facade.Config.Set("k8s.default_namespace", "default")
}

func TestParseTrafficParamsDoesNotAllowNormalUserNamespaceOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTrafficTestConfig()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/panel-api/v1/traffic/pods?namespace=other", nil)
	ctx.Set("user_mode", "normal")

	params, ok := parseTrafficParams(ctx)
	if !ok {
		t.Fatal("expected valid params")
	}
	if params.Namespace == "other" || params.Namespace == "*" {
		t.Fatalf("normal user escaped namespace scope: %q", params.Namespace)
	}
}

func TestParseTrafficParamsAllowsFounderAllNamespaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTrafficTestConfig()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/panel-api/v1/traffic/domains?namespace=*", nil)
	ctx.Set("user_mode", "founder")

	params, ok := parseTrafficParams(ctx)
	if !ok || params.Namespace != "*" {
		t.Fatalf("founder namespace = %q, ok=%v", params.Namespace, ok)
	}
}
