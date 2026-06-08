package webhook

import (
	"testing"

	networkingv1 "k8s.io/api/networking/v1"
)

func TestIngressRuleHostsNormalizesHosts(t *testing.T) {
	ingress := &networkingv1.Ingress{
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "Example.COM."},
				{Host: "  API.Example.COM  "},
				{Host: ""},
			},
		},
	}

	hosts := ingressRuleHosts(ingress)

	if _, ok := hosts["example.com"]; !ok {
		t.Fatal("expected example.com host")
	}
	if _, ok := hosts["api.example.com"]; !ok {
		t.Fatal("expected api.example.com host")
	}
	if _, ok := hosts[""]; ok {
		t.Fatal("expected empty host to be skipped")
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}
}
