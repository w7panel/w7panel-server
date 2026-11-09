package webhook

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestWildcardIngressRequiresDNSForAutomaticIssuer(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		issuer  string
		allowed bool
	}{
		{"", true}, {"w7-letsencrypt-prod", false},
	} {
		ingress := testIngress("wildcard", "default", "*.example.com")
		ingress.Spec.TLS = []networkingv1.IngressTLS{{Hosts: []string{"*.example.com"}, SecretName: "x-example-com-tls"}}
		if tc.issuer != "" {
			ingress.Annotations = map[string]string{"cert-manager.io/cluster-issuer": tc.issuer}
		}
		raw, err := json.Marshal(ingress)
		if err != nil {
			t.Fatal(err)
		}
		mutator := NewResourceMutator(fake.NewClientBuilder().WithScheme(scheme).Build(), nil)
		response := mutator.handleIngress(context.Background(), admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Create, Object: runtime.RawExtension{Raw: raw},
		}})
		if response.Allowed != tc.allowed {
			t.Fatalf("issuer %q allowed=%v, want %v: %v", tc.issuer, response.Allowed, tc.allowed, response.Result)
		}
	}
}

func TestFindIngressHostConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	tests := []struct {
		name      string
		existing  []runtime.Object
		current   *networkingv1.Ingress
		wantBlock bool
	}{
		{
			name: "blocks host used by another namespace",
			existing: []runtime.Object{
				testIngress("site", "team-a", "example.com"),
			},
			current:   testIngress("site", "team-b", "example.com"),
			wantBlock: true,
		},
		{
			name: "allows duplicate host in same namespace",
			existing: []runtime.Object{
				testIngress("site-a", "team-a", "example.com"),
			},
			current:   testIngress("site-b", "team-a", "example.com"),
			wantBlock: false,
		},
		{
			name: "allows current ingress host on update",
			existing: []runtime.Object{
				testIngress("site", "team-a", "example.com"),
			},
			current:   testIngress("site", "team-a", "example.com"),
			wantBlock: false,
		},
		{
			name: "ignores empty hosts",
			existing: []runtime.Object{
				testIngress("site", "team-a", "example.com"),
			},
			current:   testIngress("site", "team-b", ""),
			wantBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existing...).Build()
			conflict, err := findIngressHostConflict(context.Background(), client, tt.current)
			if err != nil {
				t.Fatalf("findIngressHostConflict() error = %v", err)
			}
			if gotBlock := conflict != ""; gotBlock != tt.wantBlock {
				t.Fatalf("findIngressHostConflict() blocked = %v, want %v, message %q", gotBlock, tt.wantBlock, conflict)
			}
		})
	}
}

func testIngress(name, namespace string, hosts ...string) *networkingv1.Ingress {
	rules := make([]networkingv1.IngressRule, 0, len(hosts))
	for _, host := range hosts {
		rules = append(rules, networkingv1.IngressRule{Host: host})
	}
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: rules,
		},
	}
}
