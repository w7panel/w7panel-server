package oidc

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestKubeDynamicClientStoreLive(t *testing.T) {
	os.Setenv("W7_OIDC_LIVE_TEST", "true")
	if os.Getenv("W7_OIDC_LIVE_TEST") != "true" {
		t.Skip("set W7_OIDC_LIVE_TEST=true to run against a real Kubernetes cluster")
	}

	sdk := k8s.NewK8sClient().Sdk
	if _, err := sdk.ClientSet.CoreV1().Namespaces().Get(sdk.Ctx, sdk.GetNamespace(), metav1.GetOptions{}); err != nil {
		t.Fatalf("failed to connect to cluster namespace %q: %v", sdk.GetNamespace(), err)
	}

	store := kubeDynamicClientStore{}
	req := DynamicClientRequest{
		RedirectURIs:          []string{"https://client.example/callback"},
		TokenEndpointAuthMode: "client_secret_basic",
		Scope:                 "openid profile offline_access",
		ClientName:            "live-test-client",
	}

	client, err := store.Create(req)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(client.ClientID); err != nil {
			t.Fatalf("cleanup delete returned error: %v", err)
		}
	})

	if client.ClientID == "" {
		t.Fatal("expected generated client_id")
	}
	if client.ClientSecret == "" {
		t.Fatal("expected generated client_secret")
	}

	secret, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, secretNameForClientID(client.ClientID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected created secret, got error: %v", err)
	}
	if got := string(secret.Data["client_secret"]); got != client.ClientSecret {
		t.Fatalf("unexpected stored client_secret: %q", got)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	found := false
	for _, item := range loaded {
		if item.ClientID == client.ClientID {
			found = true
			if item.TokenEndpointAuthMode != "client_secret_basic" {
				t.Fatalf("unexpected auth mode from Load: %s", item.TokenEndpointAuthMode)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected client %s in Load result", client.ClientID)
	}

	client.Name = "live-test-client-updated"
	client.RedirectURIs = []string{"https://client.example/updated"}
	if err := store.Save(client, true); err != nil {
		t.Fatalf("Save(update) returned error: %v", err)
	}

	updatedSecret, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, secretNameForClientID(client.ClientID), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected updated secret, got error: %v", err)
	}
	if got := string(updatedSecret.Data["client_name"]); got != client.Name {
		t.Fatalf("unexpected updated client_name: %q", got)
	}

	if err := store.Delete(client.ClientID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if _, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, secretNameForClientID(client.ClientID), metav1.GetOptions{}); !k8serrors.IsNotFound(err) {
		t.Fatalf("expected secret to be deleted, got err=%v", err)
	}
}
