package metrics

import "testing"

func TestMetricsServiceName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "legacy", version: "1.0.22", want: legacyMetricsService},
		{name: "baseline", version: "1.0.23", want: metricsService},
		{name: "newer", version: "v1.1.0", want: metricsService},
		{name: "empty version", version: "", want: legacyMetricsService},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MetricsServiceName(tt.version); got != tt.want {
				t.Fatalf("MetricsServiceName(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestLegacyMetricsQueryReplacement(t *testing.T) {
	query := `rate(node_cpu_usage_seconds_total{job="default/w7panel-metrics-node-resource"}) + node_disk_reads_completed_total{job="default/w7panel-metrics-node-exporter"}`
	want := `rate(node_cpu_usage_seconds_total{job="default/w7panel-metrics-k8s-offline-metrics-node-resource"}) + node_disk_reads_completed_total{job="default/w7panel-metrics-k8s-offline-metrics-node-exporter"}`
	if got := rewriteLegacyQuery(query); got != want {
		t.Fatalf("rewriteLegacyQuery() = %q, want %q", got, want)
	}
}
