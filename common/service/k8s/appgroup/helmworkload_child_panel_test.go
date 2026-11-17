package appgroup

import (
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

func TestChildPanelAppGroupUsesRunningImageVersion(t *testing.T) {
	installed := &release.Release{
		Name: "child-panel", Namespace: "default",
		Chart:  &chart.Chart{Metadata: &chart.Metadata{AppVersion: "1.1.110", Annotations: map[string]string{"w7.cc/identifie": "w7panel-offline"}}},
		Config: map[string]interface{}{"childPanel": map[string]interface{}{"enabled": true, "imageTag": "1.1.109"}},
	}
	if got := (&HelmWorkload{}).releaseToAppGroup(installed).Spec.Version; got != "1.1.109" {
		t.Fatalf("child panel version = %q", got)
	}
	installed.Config = map[string]interface{}{
		"childPanel": map[string]interface{}{"enabled": true},
		"image":      map[string]interface{}{"tag": "1.1.111"},
	}
	if got := (&HelmWorkload{}).releaseToAppGroup(installed).Spec.Version; got != "1.1.111" {
		t.Fatalf("child panel image.tag version = %q", got)
	}
	installed.Config = nil
	if got := (&HelmWorkload{}).releaseToAppGroup(installed).Spec.Version; got != "1.1.110" {
		t.Fatalf("regular panel version = %q", got)
	}
}
