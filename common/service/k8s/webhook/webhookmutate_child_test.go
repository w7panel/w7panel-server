package webhook

import "testing"

func TestChildAgentWebhookSkipsHostWorkloads(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	for _, rule := range getOperations() {
		for _, resource := range rule.Resources {
			switch resource {
			case "deployments", "daemonsets", "statefulsets", "services", "configmaps":
				t.Fatalf("child webhook must not watch host resource %q", resource)
			}
		}
	}
}
