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
	ctx.Set("username", "minghu")

	params, ok := parseTrafficParams(ctx)
	if !ok {
		t.Fatal("expected valid params")
	}
	if params.Namespace != "k3k-minghu" {
		t.Fatalf("normal user namespace = %q, want k3k-minghu", params.Namespace)
	}
}

func TestParseTrafficParamsAllowsFounderAllNamespaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTrafficTestConfig()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/panel-api/v1/traffic/domains?namespace=*&domain=demo.example.com&upstreamIp=10.42.0.8&search=checkout", nil)
	ctx.Set("user_mode", "founder")

	params, ok := parseTrafficParams(ctx)
	if !ok || params.Namespace != "*" {
		t.Fatalf("founder namespace = %q, ok=%v", params.Namespace, ok)
	}
	if params.Domain != "demo.example.com" || params.UpstreamIP != "10.42.0.8" || params.Search != "checkout" {
		t.Fatalf("filter params were not parsed: %#v", params)
	}
}

func TestParseTrafficParamsReadsWorkloadFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initTrafficTestConfig()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/panel-api/v1/traffic/apps?workloadKind=CronJob&workloadName=cleanup", nil)
	ctx.Set("user_mode", "founder")
	params, ok := parseTrafficParams(ctx)
	if !ok || params.WorkloadKind != "CronJob" || params.WorkloadName != "cleanup" {
		t.Fatalf("workload filters were not parsed: %#v", params)
	}
}
