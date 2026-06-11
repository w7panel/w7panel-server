package metrics

import "testing"

func TestNodeScrapeParse(t *testing.T) {
	scrape := NewNodeScrape("127.0.0.1", HAMIPORT)
	data := `# HELP HostCoreUtilization GPU core utilization
# TYPE HostCoreUtilization gauge
HostCoreUtilization{deviceidx="0",deviceuuid="GPU-1",zone="vGPU"} 0
`

	metrics, err := scrape.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(metrics) == 0 {
		t.Fatal("Parse() returned no metric families")
	}

	if _, ok := metrics["HostCoreUtilization"]; !ok {
		t.Fatal("Parse() missing HostCoreUtilization metric family")
	}
}
