package apiclient

import (
	"context"
	"testing"
	"time"

	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetCachedApiClientUsesMemoryCache(t *testing.T) {
	cache := newApiClientCache(time.Hour)
	loadCount := 0

	loader := func(_ context.Context, namespace, name string) (*apiclientv1alpha1.ApiClient, error) {
		loadCount++
		return &apiclientv1alpha1.ApiClient{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
			},
			Spec: apiclientv1alpha1.ApiClientSpec{
				ClientID:   name,
				ClientName: "demo-client",
			},
		}, nil
	}

	first, err := cache.get(context.Background(), "default", "client-1", loader)
	if err != nil {
		t.Fatalf("first cache get failed: %v", err)
	}
	second, err := cache.get(context.Background(), "default", "client-1", loader)
	if err != nil {
		t.Fatalf("second cache get failed: %v", err)
	}

	if loadCount != 1 {
		t.Fatalf("expected loader to be called once, got %d", loadCount)
	}
	if first == second {
		t.Fatal("expected cache get to return deep copies")
	}
	if second.Spec.ClientName != "demo-client" {
		t.Fatalf("unexpected client name: %s", second.Spec.ClientName)
	}
}

func TestGetCachedApiClientByIDUsesNamespaceCache(t *testing.T) {
	cache := newApiClientCache(time.Hour)
	loadCount := 0

	loader := func(_ context.Context, namespace string) ([]apiclientv1alpha1.ApiClient, error) {
		loadCount++
		return []apiclientv1alpha1.ApiClient{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      "api-client-a",
				},
				Spec: apiclientv1alpha1.ApiClientSpec{
					ClientID:   "client-1",
					ClientName: "demo-client",
				},
			},
		}, nil
	}

	first, err := cache.getByClientID(context.Background(), "default", "client-1", loader)
	if err != nil {
		t.Fatalf("first cache get failed: %v", err)
	}
	second, err := cache.getByClientID(context.Background(), "default", "client-1", loader)
	if err != nil {
		t.Fatalf("second cache get failed: %v", err)
	}

	if loadCount != 1 {
		t.Fatalf("expected loader to be called once, got %d", loadCount)
	}
	if first == nil || second == nil {
		t.Fatal("expected cached client")
	}
	if first.Name != "api-client-a" || second.Spec.ClientID != "client-1" {
		t.Fatalf("unexpected cached client: %#v %#v", first, second)
	}
}

func TestApiClientCacheDeleteClearsPendingAccess(t *testing.T) {
	cache := newApiClientCache(time.Hour)
	cache.upsert(&apiclientv1alpha1.ApiClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "api-client-a",
		},
		Spec: apiclientv1alpha1.ApiClientSpec{
			ClientID: "client-1",
		},
	})
	cache.markAccessed("default", "api-client-a", time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC))
	cache.delete("default", "api-client-a")

	pending := cache.snapshotPending()
	if len(pending) != 0 {
		t.Fatalf("expected no pending access after delete, got %d", len(pending))
	}
}

func TestApiClientCacheMarkAccessedUpdatesStatusCache(t *testing.T) {
	cache := newApiClientCache(time.Hour)
	cache.upsert(&apiclientv1alpha1.ApiClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "api-client-a",
		},
		Spec: apiclientv1alpha1.ApiClientSpec{
			ClientID: "client-1",
		},
	})

	accessTime := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	cache.markAccessed("default", "api-client-a", accessTime)

	client := cache.getCached("default", "api-client-a")
	if client == nil || client.Status.LastAccessedAt == nil {
		t.Fatal("expected cached last accessed time")
	}
	if !client.Status.LastAccessedAt.Time.Equal(accessTime) {
		t.Fatalf("unexpected last accessed time: %v", client.Status.LastAccessedAt.Time)
	}
}

func TestNormalizeTokenType(t *testing.T) {
	if got := NormalizeTokenType(""); got != apiclientv1alpha1.TokenTypeTemporary {
		t.Fatalf("empty token type = %q, want temporary", got)
	}
	if got := NormalizeTokenType(apiclientv1alpha1.TokenTypePermanent); got != apiclientv1alpha1.TokenTypePermanent {
		t.Fatalf("permanent token type = %q, want permanent", got)
	}
	if IsValidTokenType("forever") {
		t.Fatal("unexpected valid custom token type")
	}
}

func TestTemporaryTokenMinutes(t *testing.T) {
	custom := int64(30)
	tooSmall := int64(0)
	tooLarge := int64(2000)
	if got := TemporaryTokenMinutes(nil); got != DefaultTemporaryTokenMinutes {
		t.Fatalf("nil temporary token minutes = %d, want %d", got, DefaultTemporaryTokenMinutes)
	}
	if got := TemporaryTokenMinutes(&custom); got != custom {
		t.Fatalf("custom temporary token minutes = %d, want %d", got, custom)
	}
	if got := TemporaryTokenMinutes(&tooSmall); got != MinTemporaryTokenMinutes {
		t.Fatalf("too small temporary token minutes = %d, want %d", got, MinTemporaryTokenMinutes)
	}
	if got := TemporaryTokenMinutes(&tooLarge); got != MaxTemporaryTokenMinutes {
		t.Fatalf("too large temporary token minutes = %d, want %d", got, MaxTemporaryTokenMinutes)
	}
	if got := TemporaryTokenSeconds(&custom); got != 1800 {
		t.Fatalf("temporary token seconds = %d, want 1800", got)
	}
}

func TestPermanentTokenSecretNameIsStable(t *testing.T) {
	first := permanentTokenSecretName("appid-demo")
	second := permanentTokenSecretName("appid-demo")
	third := permanentTokenSecretName("appid-other")
	if first != second {
		t.Fatal("expected stable permanent token secret name")
	}
	if first == third {
		t.Fatal("expected different appid to use different permanent token secret name")
	}
	if len(first) > 63 {
		t.Fatalf("secret name is too long: %s", first)
	}
}
