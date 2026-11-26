// nolint
package zpk

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/types"
	corev1 "k8s.io/api/core/v1"
)

type loadBalancerManifest struct{ K8sResourceInterface }

func (loadBalancerManifest) GetName() string                        { return "test" }
func (loadBalancerManifest) GetNamespace() string                   { return "default" }
func (loadBalancerManifest) GetLabels() map[string]string           { return nil }
func (loadBalancerManifest) GetAnnotations() map[string]string      { return nil }
func (loadBalancerManifest) GetMatchLabels() map[string]string      { return nil }
func (loadBalancerManifest) GetServiceLbPort() []corev1.ServicePort { return nil }

func TestToLoadBalancerServiceClass(t *testing.T) {
	for _, tc := range []struct {
		name, isChild, wantClass string
	}{
		{"main panel", "false", "io.cilium/node"},
		{"child panel", "true", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IS_CHILD", tc.isChild)
			service := ToLoadBalancerService(loadBalancerManifest{})
			if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
				t.Fatalf("type = %q", service.Spec.Type)
			}
			if tc.wantClass == "" {
				if service.Spec.LoadBalancerClass != nil {
					t.Fatalf("loadBalancerClass = %q, want unset", *service.Spec.LoadBalancerClass)
				}
			} else if service.Spec.LoadBalancerClass == nil || *service.Spec.LoadBalancerClass != tc.wantClass {
				t.Fatalf("loadBalancerClass = %v, want %q", service.Spec.LoadBalancerClass, tc.wantClass)
			}
		})
	}
}

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
