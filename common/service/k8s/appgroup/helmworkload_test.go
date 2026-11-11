package appgroup

import (
	"testing"

	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func disabledRelease(t *testing.T) {
	sdk := k8s.NewK8sClient()
	api := helmservice.NewHelm(sdk.Sdk)

	releases, err := api.ListRaw("default", "")
	if err != nil {
		t.Errorf("error: %v", err)
	}
	for _, release := range releases {
		t.Logf("release: %v", release)
		anno := release.Chart.Metadata.Annotations
		t.Logf("annotations: %v", anno)
	}
}
