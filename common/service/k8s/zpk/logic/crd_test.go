package logic

import (
	"strings"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

func TestUpgradeCRDsRespectDisabledSubchart(t *testing.T) {
	makeChart := func() *chart.Chart {
		parent := &chart.Chart{
			Metadata: &chart.Metadata{Name: "parent", Version: "1.0.0", Dependencies: []*chart.Dependency{{
				Name: "child", Version: "1.0.0", Condition: "child.enabled",
			}}},
			Values: map[string]interface{}{"child": map[string]interface{}{"enabled": true}},
		}
		child := &chart.Chart{
			Metadata: &chart.Metadata{Name: "child", Version: "1.0.0"},
			Files:    []*chart.File{{Name: "crds/widgets.yaml", Data: []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: widgets.example.com\n")}},
		}
		parent.SetDependencies(child)
		return parent
	}
	disabledChart := makeChart()
	root := &types.PackageApp{ManifestPackage: &types.ManifestPackage{}, InstallOption: &types.InstallOption{}}
	root.Manifest.Platform.StartParams = []types.StartParams{{Name: "child.enabled", ValuesText: "false"}}
	assignments := helmSetAssignments(root, "", []string{"HELM_ATOMIC", "DOMAIN_URL"}, false)
	if !strings.Contains(fillHelmSet(root, "", []string{"HELM_ATOMIC", "DOMAIN_URL"}, false), "--set 'child.enabled=false'") {
		t.Fatal("install Job did not receive the dependency condition")
	}
	_, disabled, err := upgradeCRDsFromChart(disabledChart, assignments)
	if err != nil || len(disabled) != 0 {
		t.Fatalf("disabled subchart CRD was included: %v, %v", disabled, err)
	}
	if err := chartutil.ProcessDependenciesWithMerge(disabledChart, map[string]interface{}{"child": map[string]interface{}{"enabled": "false"}}); err != nil || len(disabledChart.CRDObjects()) != 0 {
		t.Fatalf("Helm reintroduced a disabled subchart: %v, %v", disabledChart.CRDObjects(), err)
	}
	_, enabled, err := upgradeCRDsFromChart(makeChart(), []string{"child.enabled=true"})
	if err != nil || enabled["widgets.example.com"] == nil {
		t.Fatalf("enabled subchart CRD was missing: %v, %v", enabled, err)
	}
}

func TestValidateRootInstallOption(t *testing.T) {
	pack := &types.ManifestPackage{}
	pack.Manifest.Application.Identifie = "root"
	if err := validateRootInstallOption(pack, []types.InstallOption{{Identifie: "child"}}); err == nil {
		t.Fatal("missing root option accepted")
	}
	if err := validateRootInstallOption(pack, []types.InstallOption{{Identifie: "root"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRequireUpgradeCRDApproval(t *testing.T) {
	result := UpgradeCRDResult{Names: []string{"widgets.example.com"}, Digest: "approved"}
	if err := requireUpgradeCRDApproval(result, ""); err == nil {
		t.Fatal("unapproved CRD update accepted")
	}
	if err := requireUpgradeCRDApproval(result, "stale"); err == nil {
		t.Fatal("stale Chart approval accepted")
	}
	if err := requireUpgradeCRDApproval(result, "approved"); err != nil {
		t.Fatal(err)
	}
	result.Names = nil
	if err := requireUpgradeCRDApproval(result, ""); err != nil {
		t.Fatal(err)
	}
}
