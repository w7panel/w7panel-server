package metrics

import (
	"context"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"golang.org/x/mod/semver"
)

const (
	metricsNamespace       = "default"
	metricsAppGroup        = "w7panel-metrics"
	metricsService         = "vmsingle-w7panel-metrics-single"
	legacyMetricsService   = "vmsingle-w7panel-metrics-k8s-offline-metrics-single"
	metricsServicePort     = "8429"
	metricsVersionBaseline = "v1.0.23"
)

// MetricsServiceName returns the VictoriaMetrics service used by a metrics chart version.
func MetricsServiceName(version string) string {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if semver.Compare("v"+version, metricsVersionBaseline) < 0 {
		return legacyMetricsService
	}
	return metricsService
}

// QueryRange queries VictoriaMetrics through the Kubernetes service proxy.
func QueryRange(ctx context.Context, sdk *k8s.Sdk, params map[string]string) ([]byte, error) {
	return query(ctx, sdk, "prometheus/api/v1/query_range", params)
}

// Query queries the current VictoriaMetrics value through the Kubernetes service proxy.
func Query(ctx context.Context, sdk *k8s.Sdk, params map[string]string) ([]byte, error) {
	return query(ctx, sdk, "prometheus/api/v1/query", params)
}

func query(ctx context.Context, sdk *k8s.Sdk, path string, params map[string]string) ([]byte, error) {
	serviceName := metricsService
	if group, err := appgroup.GetAppgroupUseSdk(metricsAppGroup, metricsNamespace, sdk); err == nil {
		serviceName = MetricsServiceName(group.Spec.Version)
	}
	if serviceName == legacyMetricsService {
		params = cloneQueryParams(params)
		params["query"] = rewriteLegacyQuery(params["query"])
	}

	return sdk.ClientSet.CoreV1().Services(metricsNamespace).
		ProxyGet("http", serviceName, metricsServicePort, path, params).
		DoRaw(ctx)
}

func rewriteLegacyQuery(query string) string {
	return strings.NewReplacer(
		"default/w7panel-metrics-node-resource", "default/w7panel-metrics-k8s-offline-metrics-node-resource",
		"w7panel-metrics-node-exporter", "w7panel-metrics-k8s-offline-metrics-node-exporter",
	).Replace(query)
}

func cloneQueryParams(params map[string]string) map[string]string {
	result := make(map[string]string, len(params))
	for key, value := range params {
		result[key] = value
	}
	return result
}
