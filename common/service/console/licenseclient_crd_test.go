package console

import (
	"crypto/x509"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLicenseSpecEncodesCertificateRawDER(t *testing.T) {
	spec := licenseCRDSpec(&License{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
		License:       &x509.Certificate{Raw: []byte("cert")},
	})

	if spec.AppId != "app" {
		t.Fatalf("AppId = %q", spec.AppId)
	}
	if spec.AppSecret != "secret" {
		t.Fatalf("AppSecret = %q", spec.AppSecret)
	}
	if spec.FounderSaName != "admin" {
		t.Fatalf("FounderSaName = %q", spec.FounderSaName)
	}
	if spec.License != "Y2VydA==" {
		t.Fatalf("License = %q, want base64 DER", spec.License)
	}
}

func TestLicenseSpecOmitsNilCertificate(t *testing.T) {
	spec := licenseCRDSpec(&License{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
	})

	if spec.License != "" {
		t.Fatalf("License = %q, want empty", spec.License)
	}
}

func TestUserLicenseSpecRoundTrip(t *testing.T) {
	want := k8s.LicenseCRDSpec{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
		License:       "Y2VydA==",
	}
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"license": userLicenseSpecMap(want),
		},
	}}

	got, ok, err := userLicenseSpec(obj)
	if err != nil {
		t.Fatalf("userLicenseSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("userLicenseSpec() ok = false, want true")
	}
	if got != want {
		t.Fatalf("userLicenseSpec() = %#v, want %#v", got, want)
	}
}
