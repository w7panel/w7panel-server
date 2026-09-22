package controller

import (
	"testing"

	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDecodeCopilotObject(t *testing.T) {
	object, err := decodeCopilotObject("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\n  namespace: default\ndata:\n  key: value\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceRef(object); got != "ConfigMap/default/example" {
		t.Fatalf("resourceRef() = %q", got)
	}
}

func TestCopilotEventTextKeepsOnlyTextParts(t *testing.T) {
	event := &session.Event{LLMResponse: modelResponse("answer", genai.RoleModel)}
	if got := copilotEventText(event); got != "answer" {
		t.Fatalf("copilotEventText() = %q", got)
	}
	if got := copilotEventText(nil); got != "" {
		t.Fatalf("copilotEventText(nil) = %q", got)
	}
}

func TestNamespaceOrDefault(t *testing.T) {
	if got := namespaceOrDefault("selected", "default"); got != "selected" {
		t.Fatalf("namespaceOrDefault() = %q", got)
	}
	if got := namespaceOrDefault("", "default"); got != "default" {
		t.Fatalf("namespaceOrDefault() = %q", got)
	}
}

func TestCopilotKubectlArgsRejectsShellAndSecrets(t *testing.T) {
	args, err := copilotKubectlArgs("kubectl get pods -n default")
	if err != nil || len(args) != 5 || args[0] != "kubectl" {
		t.Fatalf("copilotKubectlArgs() = %#v, %v", args, err)
	}
	for _, command := range []string{"kubectl get secrets", "kubectl get pods; id", "helm list"} {
		if _, err := copilotKubectlArgs(command); err == nil {
			t.Fatalf("copilotKubectlArgs(%q) succeeded", command)
		}
	}
}

func TestDecodeCopilotObjectRejectsMultipleResources(t *testing.T) {
	_, err := decodeCopilotObject("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: first\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: second\n")
	if err == nil {
		t.Fatal("expected multiple manifests to be rejected")
	}
}

func TestSummarizeCopilotPodPrioritizesFailingContainer(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api"}}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{
			{Name: "api", RestartCount: 2, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			{Name: "sidecar", RestartCount: 5},
		}},
	}
	summary := summarizeCopilotPod(pod)
	if summary.Container != "api" || summary.Reason != "CrashLoopBackOff" || summary.Priority != 100 {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if summary.Workload != "Deployment/api" || summary.Restarts != 7 {
		t.Fatalf("unexpected workload or restarts: %#v", summary)
	}
}

func TestAbnormalCopilotPodsKeepsOnlyThreeHighestPriority(t *testing.T) {
	pods := abnormalCopilotPods([]copilotPodSummary{
		{Name: "normal"},
		{Name: "pending", Priority: 60},
		{Name: "image", Priority: 90},
		{Name: "crash", Priority: 100},
		{Name: "restart", Priority: 40, Restarts: 99},
	})
	if len(pods) != 3 || pods[0].Name != "crash" || pods[1].Name != "image" || pods[2].Name != "pending" {
		t.Fatalf("unexpected abnormal pods: %#v", pods)
	}
}

func TestLimitCopilotPodsPrefersFailures(t *testing.T) {
	pods := limitCopilotPods([]copilotPodSummary{
		{Name: "normal-b"},
		{Name: "restart", Priority: 40, Restarts: 2},
		{Name: "crash", Priority: 100},
		{Name: "normal-a"},
	}, 2)
	if len(pods) != 2 || pods[0].Name != "crash" || pods[1].Name != "restart" {
		t.Fatalf("unexpected limited pods: %#v", pods)
	}
}
