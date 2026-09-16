// nolint
package zpk

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/types"
)

type mockBuildImageOption struct {
	types.BuildImageInterface
}

func TestToZpkBuildCronJobUsesServiceAccountForLocalRegistry(t *testing.T) {
	params := &types.BuildImageParams{
		DockerRegistry:     types.DockerRegistry{Host: "registry.local.w7.cc", Namespace: "default"},
		DockerfilePath:     "Dockerfile",
		BuildContext:       "/workspace/",
		PushImage:          "registry.local.w7.cc/default/test:latest",
		ZipUrl:             "https://example.invalid/Dockerfile.zip",
		Identifie:          "test",
		Title:              "test1",
		Labels:             map[string]string{"test": "test"},
		BuildJobName:       "test1",
		ServiceAccountName: "founder",
	}

	cronjob := ToZpkBuildCronJob(params, "*/1 * * * *")
	pod := cronjob.Spec.JobTemplate.Spec.Template.Spec
	if pod.ServiceAccountName != "founder" {
		t.Fatalf("serviceAccountName = %q", pod.ServiceAccountName)
	}
	if pod.AutomountServiceAccountToken == nil || !*pod.AutomountServiceAccountToken {
		t.Fatal("local registry cronjob must mount its ServiceAccount token")
	}
	for _, env := range pod.Containers[0].Env {
		if env.Name == "REGISTRY_SERVICE_ACCOUNT_TOKEN_FILE" {
			return
		}
	}
	t.Fatal("local registry cronjob must receive the ServiceAccount token path")
}
