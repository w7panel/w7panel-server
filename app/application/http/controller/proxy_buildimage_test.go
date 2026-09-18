package controller

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindBuildImageServiceAccount(t *testing.T) {
	req := httptest.NewRequest("POST", "/apis/w7panel.w7.com/v1alpha1/namespaces/default/buildimages", strings.NewReader(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"BuildImage","spec":{"serviceAccountName":"attacker"}}`))
	if err := bindBuildImageServiceAccount(req, req.URL.Path, "founder"); err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"serviceAccountName":"founder"`) {
		t.Fatalf("unexpected patched body: %s", body)
	}
}

func TestBindBuildImageRejectsDefaultIdentity(t *testing.T) {
	req := httptest.NewRequest("POST", "/apis/w7panel.w7.com/v1alpha1/namespaces/default/buildimages", strings.NewReader(`{"apiVersion":"w7panel.w7.com/v1alpha1","kind":"BuildImage","spec":{}}`))
	if err := bindBuildImageServiceAccount(req, req.URL.Path, "default"); err == nil {
		t.Fatal("default ServiceAccount must be rejected")
	}
}
