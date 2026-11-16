package console

import (
	"testing"

	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"helm.sh/helm/v3/pkg/chart"
)

func TestVerifyCRDApproval(t *testing.T) {
	empty := &chart.Chart{Metadata: &chart.Metadata{Name: "app", Version: "1.0.0"}}
	emptyCRDs, err := helmservice.ChartCRDs(empty)
	if err != nil {
		t.Fatal(err)
	}
	emptyDigest, err := helmservice.CRDDigest(emptyCRDs)
	if err != nil || emptyDigest == "" || verifyCRDApproval(empty, emptyDigest) != nil {
		t.Fatalf("empty CRD chart must keep a valid approval digest: %q, %v", emptyDigest, err)
	}
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "app", Version: "1.0.0"}, Files: []*chart.File{{Name: "crds/widgets.yaml", Data: []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: widgets.example.com\nspec:\n  group: example.com\n")}}}
	crds, err := helmservice.ChartCRDs(ch)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := helmservice.CRDDigest(crds)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyCRDApproval(ch, digest); err != nil {
		t.Fatal(err)
	}
	ch.Metadata.Version = "2.0.0"
	if err := verifyCRDApproval(ch, digest); err != nil {
		t.Fatal("Chart metadata changed the CRD approval")
	}
	ch.Files[0].Data = []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: widgets.example.com\nspec:\n  group: changed.example.com\n")
	if err := verifyCRDApproval(ch, digest); err == nil {
		t.Fatal("changed CRD accepted")
	}
}
