package console

import (
	"crypto/x509"
	"testing"
)

func TestLicenseCRDSpecEncodesCertificateRawDER(t *testing.T) {
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

func TestLicenseCRDSpecOmitsNilCertificate(t *testing.T) {
	spec := licenseCRDSpec(&License{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
	})

	if spec.License != "" {
		t.Fatalf("License = %q, want empty", spec.License)
	}
}
