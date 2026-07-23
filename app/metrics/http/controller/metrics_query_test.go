package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
)

func TestShouldUseRootMetricsSDK(t *testing.T) {
	tests := []struct {
		name        string
		isK3k       bool
		clusterMode string
		want        bool
	}{
		{name: "standalone", isK3k: false, want: true},
		{name: "shared", isK3k: true, clusterMode: k3ktypes.K3K_CLUSTER_MODE_SHARED, want: true},
		{name: "virtual", isK3k: true, clusterMode: k3ktypes.K3K_CLUSTER_MODE_VIRTUAL, want: false},
		{name: "k3k defaults to child", isK3k: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUseRootMetricsSDK(tt.isK3k, tt.clusterMode); got != tt.want {
				t.Fatalf("shouldUseRootMetricsSDK(%v, %q) = %v, want %v", tt.isK3k, tt.clusterMode, got, tt.want)
			}
		})
	}
}

func TestMetricsQueryRange(t *testing.T) {
	originalResolver := resolveMetricsSDK
	originalQuery := queryMetricsRange
	t.Cleanup(func() {
		resolveMetricsSDK = originalResolver
		queryMetricsRange = originalQuery
	})

	var gotForceLocal bool
	resolveMetricsSDK = func(token string, forceLocal bool) (*k8s.Sdk, error) {
		if token != "test-token" {
			t.Fatalf("token = %q, want test-token", token)
		}
		gotForceLocal = forceLocal
		return &k8s.Sdk{}, nil
	}
	var gotParams map[string]string
	queryMetricsRange = func(_ context.Context, _ *k8s.Sdk, params map[string]string) ([]byte, error) {
		gotParams = params
		return []byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`), nil
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/panel-api/v1/metrics/query-range", func(ctx *gin.Context) {
		ctx.Set("k8s_token", "test-token")
	}, Metrics{}.QueryRange)
	params := url.Values{
		"query": {"node_cpu_usage_seconds_total"},
		"start": {"100"},
		"end":   {"200"},
		"step":  {"15"},
		"local": {"1"},
	}
	request := httptest.NewRequest(http.MethodGet, "/panel-api/v1/metrics/query-range?"+params.Encode(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !gotForceLocal {
		t.Fatal("local=1 must force the root cluster SDK")
	}
	if gotParams["query"] != params.Get("query") || gotParams["start"] != "100" || gotParams["end"] != "200" || gotParams["step"] != "15" {
		t.Fatalf("forwarded params = %#v", gotParams)
	}
	if _, exists := gotParams["local"]; exists {
		t.Fatal("local must not be forwarded to VictoriaMetrics")
	}
}

func TestMetricsQueryRangeRequiresQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/metrics/query-range", nil)

	Metrics{}.QueryRange(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
