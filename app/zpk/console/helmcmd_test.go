package console

import (
	"testing"

	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"helm.sh/helm/v3/pkg/chart"
)

func TestVerifyChartApproval(t *testing.T) {
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "app", Version: "1.0.0"}}
	digest, err := helmservice.ChartDigest(ch)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyChartApproval(ch, digest); err != nil {
		t.Fatal(err)
	}
	ch.Metadata.Version = "2.0.0"
	if err := verifyChartApproval(ch, digest); err == nil {
		t.Fatal("changed Chart accepted")
	}
}
