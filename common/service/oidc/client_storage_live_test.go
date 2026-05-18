package oidc

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	oidcv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/oidc/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestKubeDynamicClientStoreLive(t *testing.T) {
	if os.Getenv("W7_OIDC_LIVE_TEST") != "true" {
		t.Skip("set W7_OIDC_LIVE_TEST=true to run against a real Kubernetes cluster")
	}

	sdk := k8s.NewK8sClient().Sdk
	if _, err := sdk.ClientSet.CoreV1().Namespaces().Get(sdk.Ctx, sdk.GetNamespace(), metav1.GetOptions{}); err != nil {
		t.Fatalf("failed to connect to cluster namespace %q: %v", sdk.GetNamespace(), err)
	}
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		t.Fatalf("failed to create controller-runtime client: %v", err)
	}

	store := kubeDynamicClientStore{}
	req := DynamicClientRequest{
		RedirectURIs:          []string{"https://client.example/callback"},
		TokenEndpointAuthMode: "client_secret_basic",
		Scope:                 "openid profile offline_access",
		ClientName:            "live-test-client",
	}

	dynamicClient, err := store.Create(req)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Delete(dynamicClient.ClientID); err != nil {
			t.Fatalf("cleanup delete returned error: %v", err)
		}
	})

	if dynamicClient.ClientID == "" {
		t.Fatal("expected generated client_id")
	}
	if dynamicClient.ClientSecret == "" {
		t.Fatal("expected generated client_secret")
	}

	oidcClient := &oidcv1alpha1.OIDCClient{}
	err = sigClient.Get(sdk.Ctx, sigclient.ObjectKey{Name: resourceNameForClientID(dynamicClient.ClientID), Namespace: sdk.GetNamespace()}, oidcClient)
	if err != nil {
		t.Fatalf("expected created oidc client, got error: %v", err)
	}
	if got := oidcClient.Spec.ClientSecret; got != dynamicClient.ClientSecret {
		t.Fatalf("unexpected stored client_secret: %q", got)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	found := false
	for _, item := range loaded {
		if item.ClientID == dynamicClient.ClientID {
			found = true
			if item.TokenEndpointAuthMode != "client_secret_basic" {
				t.Fatalf("unexpected auth mode from Load: %s", item.TokenEndpointAuthMode)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected client %s in Load result", dynamicClient.ClientID)
	}

	dynamicClient.Name = "live-test-client-updated"
	dynamicClient.RedirectURIs = []string{"https://client.example/updated"}
	if err := store.Save(dynamicClient, true); err != nil {
		t.Fatalf("Save(update) returned error: %v", err)
	}

	updatedClient := &oidcv1alpha1.OIDCClient{}
	err = sigClient.Get(sdk.Ctx, sigclient.ObjectKey{Name: resourceNameForClientID(dynamicClient.ClientID), Namespace: sdk.GetNamespace()}, updatedClient)
	if err != nil {
		t.Fatalf("expected updated oidc client, got error: %v", err)
	}
	if got := updatedClient.Spec.ClientName; got != dynamicClient.Name {
		t.Fatalf("unexpected updated client_name: %q", got)
	}

	if err := store.Delete(dynamicClient.ClientID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := sigClient.Get(sdk.Ctx, sigclient.ObjectKey{Name: resourceNameForClientID(dynamicClient.ClientID), Namespace: sdk.GetNamespace()}, &oidcv1alpha1.OIDCClient{}); !k8serrors.IsNotFound(err) {
		t.Fatalf("expected oidc client to be deleted, got err=%v", err)
	}
}
