package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestToKubeconfigForServiceAccountUsesOnlySelectedAccount(t *testing.T) {
	sdk, _ := newKubeconfigTestSDK(t, true)
	sdk.restConfig = &rest.Config{
		Host:        "https://cluster.internal:6443",
		BearerToken: "caller-token",
		Username:    "caller-user",
		Password:    "caller-password",
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   []byte("cluster-ca"),
			CertData: []byte("caller-cert"),
			KeyData:  []byte("caller-key"),
		},
	}

	config, err := sdk.ToKubeconfigForServiceAccount("https://public.example:6443", "api")
	if err != nil {
		t.Fatalf("ToKubeconfigForServiceAccount() error = %v", err)
	}
	if len(config.Clusters) != 1 || config.Clusters[0].Cluster.Server != "https://public.example:6443" {
		t.Fatalf("unexpected clusters: %#v", config.Clusters)
	}
	if string(config.Clusters[0].Cluster.CertificateAuthorityData) != "cluster-ca" {
		t.Fatalf("unexpected cluster CA data: %q", config.Clusters[0].Cluster.CertificateAuthorityData)
	}
	if config.CurrentContext != "default" {
		t.Fatalf("current context = %q, want default", config.CurrentContext)
	}
	if len(config.AuthInfos) != 1 {
		t.Fatalf("unexpected auth infos: %#v", config.AuthInfos)
	}
	auth := config.AuthInfos[0].AuthInfo
	if auth.Token != "api-token" {
		t.Fatalf("token = %q, want api-token", auth.Token)
	}
	if len(auth.ClientCertificateData) != 0 || len(auth.ClientKeyData) != 0 || auth.Username != "" || auth.Password != "" {
		t.Fatalf("caller credentials leaked into kubeconfig: %#v", auth)
	}
}

func TestToKubeconfigForServiceAccountDoesNotFallbackWhenAccountMissing(t *testing.T) {
	sdk, requestedPaths := newKubeconfigTestSDK(t, false)
	sdk.restConfig = &rest.Config{Host: "https://cluster.internal:6443", BearerToken: "caller-token"}

	_, err := sdk.ToKubeconfigForServiceAccount("", "api")
	if err == nil || !strings.Contains(err.Error(), "get service account api") {
		t.Fatalf("error = %v, want missing api ServiceAccount error", err)
	}
	for _, path := range *requestedPaths {
		if strings.Contains(path, "/secrets/") {
			t.Fatalf("unexpected fallback token secret request: %s", path)
		}
	}
}

func newKubeconfigTestSDK(t *testing.T, serviceAccountExists bool) (*Sdk, *[]string) {
	t.Helper()
	requestedPaths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPaths = append(requestedPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/namespaces/default/serviceaccounts/api":
			if !serviceAccountExists {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(metav1.Status{
					Status:  metav1.StatusFailure,
					Message: `serviceaccounts "api" not found`,
					Reason:  metav1.StatusReasonNotFound,
					Code:    http.StatusNotFound,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(corev1.ServiceAccount{
				TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ServiceAccount"},
				ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			})
		case "/api/v1/namespaces/default/secrets/api":
			_ = json.NewEncoder(w).Encode(corev1.Secret{
				TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "api",
					Namespace: "default",
					Annotations: map[string]string{
						corev1.ServiceAccountNameKey: "api",
					},
				},
				Type: corev1.SecretTypeServiceAccountToken,
				Data: map[string][]byte{corev1.ServiceAccountTokenKey: []byte("api-token")},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(metav1.Status{Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotFound, Code: http.StatusNotFound})
		}
	}))
	t.Cleanup(server.Close)
	clientSet, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return &Sdk{
		ClientSet: clientSet,
		Ctx:       context.Background(),
		namespace: "default",
	}, &requestedPaths
}
