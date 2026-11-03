package dnsprofile

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWildcardInZone(t *testing.T) {
	for _, host := range []string{"*.example.com", "*.sub.example.com"} {
		if !WildcardInZone(host, "example.com") {
			t.Fatalf("rejected %s", host)
		}
	}
	for _, host := range []string{"example.com", "*.other.com", "*.badexample.com", "*.*.example.com"} {
		if WildcardInZone(host, "example.com") {
			t.Fatalf("accepted %s", host)
		}
	}
}

func TestGrantBindsHostAndIssuer(t *testing.T) {
	key := []byte("test-only")
	base := signGrant(key, "default", "ing-one", "*.example.com", "w7-dns-one")
	for _, other := range []string{
		signGrant(key, "other", "ing-one", "*.example.com", "w7-dns-one"),
		signGrant(key, "default", "ing-two", "*.example.com", "w7-dns-one"),
		signGrant(key, "default", "ing-one", "*.other.com", "w7-dns-one"),
		signGrant(key, "default", "ing-one", "*.example.com", "w7-dns-two"),
	} {
		if base == other {
			t.Fatal("grant did not bind all resource fields")
		}
	}
}

func TestDeriveChildGrant(t *testing.T) {
	id := "0123456789abcdef01234567"
	issuer := profilePrefix + id
	key := []byte("0123456789abcdef0123456789abcdef")
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: secretNamespace, Name: issuer,
			Labels: map[string]string{"w7.cc/dns-profile": "true"}, Annotations: map[string]string{"w7.cc/dns-zone": "example.com"}}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: secretNamespace, Name: grantKeyName}, Data: map[string][]byte{"key": key}},
	).Build()
	ctx := context.Background()
	signature := signGrant(key, "default", "parent-child", "*.example.com", issuer)
	grant, err := DeriveChildGrant(ctx, c, "default", "parent-child", "route-child", "*.example.com", id, signature)
	if err != nil {
		t.Fatal(err)
	}
	if grant[GrantAnnotation] != signGrant(key, "default", "route-child", "*.example.com", issuer) {
		t.Fatal("derived grant is not bound to route")
	}
	if _, err := DeriveChildGrant(ctx, c, "default", "parent-other", "route-child", "*.example.com", id, signature); err == nil {
		t.Fatal("accepted another parent")
	}
	if _, err := DeriveChildGrant(ctx, c, "default", "parent-child", "route-child", "*.other.com", id, signature); err == nil {
		t.Fatal("accepted another zone")
	}
}
