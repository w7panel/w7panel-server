package webhook

import (
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildMutatingWebhookConfigurationUsesExplicitCABundle(t *testing.T) {
	withTestSvcName(t, "w7panel")

	config := buildMutatingWebhookConfiguration(
		webhookTLSConfig{CABundle: []byte("ca")},
		"w7panel",
		"default",
		"w7panel-webhook",
		nil,
	)

	if got := config.Annotations[certManagerInjectCAFromAnnotation]; got != "" {
		t.Fatalf("unexpected cert-manager annotation %q", got)
	}
	if got := string(config.Webhooks[0].ClientConfig.CABundle); got != "ca" {
		t.Fatalf("CABundle = %q, want ca", got)
	}
}

func TestBuildMutatingWebhookConfigurationUsesCertManagerInjection(t *testing.T) {
	withTestSvcName(t, "w7panel")

	config := buildMutatingWebhookConfiguration(
		webhookTLSConfig{InjectCAFrom: "default/w7panel-webhook-tls"},
		"w7panel",
		"default",
		"w7panel-webhook",
		nil,
	)

	if got := config.Annotations[certManagerInjectCAFromAnnotation]; got != "default/w7panel-webhook-tls" {
		t.Fatalf("cert-manager annotation = %q, want default/w7panel-webhook-tls", got)
	}
	if len(config.Webhooks[0].ClientConfig.CABundle) != 0 {
		t.Fatal("expected cert-manager mode to leave CABundle empty before injection")
	}
}

func TestPreserveInjectedCABundle(t *testing.T) {
	next := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{certManagerInjectCAFromAnnotation: "default/w7panel-webhook-tls"},
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{Name: "w7panel.default.svc"},
		},
	}
	existing := &admissionregistrationv1.MutatingWebhookConfiguration{
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{
				Name: "w7panel.default.svc",
				ClientConfig: admissionregistrationv1.WebhookClientConfig{
					CABundle: []byte("injected-ca"),
				},
			},
		},
	}

	preserveInjectedCABundle(next, existing)

	if got := string(next.Webhooks[0].ClientConfig.CABundle); got != "injected-ca" {
		t.Fatalf("CABundle = %q, want injected-ca", got)
	}
}

func withTestSvcName(t *testing.T, name string) {
	t.Helper()

	old := svcName
	svcName = name
	t.Cleanup(func() {
		svcName = old
	})
}
