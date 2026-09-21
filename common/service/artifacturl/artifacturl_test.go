package artifacturl

import (
	"context"
	"net"
	"testing"
)

func withResolver(t *testing.T, ips ...string) {
	t.Helper()
	previous := lookupIP
	lookupIP = func(context.Context, string) ([]net.IPAddr, error) {
		result := make([]net.IPAddr, 0, len(ips))
		for _, raw := range ips {
			result = append(result, net.IPAddr{IP: net.ParseIP(raw)})
		}
		return result, nil
	}
	t.Cleanup(func() { lookupIP = previous })
}

func TestValidateRejectsUnsafeURLs(t *testing.T) {
	withResolver(t, "8.8.8.8")
	for _, raw := range []string{
		"http://zpk.w7.cc/index.yaml",
		"https://127.0.0.1/index.yaml",
		"https://zpk.w7.cc:8443/index.yaml",
		"https://user:pass@zpk.w7.cc/index.yaml",
		"https://example.invalid/index.yaml",
	} {
		if _, err := Validate(context.Background(), raw); err == nil {
			t.Fatalf("Validate(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestValidateRejectsAllowedHostResolvingPrivate(t *testing.T) {
	withResolver(t, "10.0.0.1")
	if _, err := Validate(context.Background(), "https://zpk.w7.cc/index.yaml"); err == nil {
		t.Fatal("expected private resolved address to be rejected")
	}
}

func TestValidateAcceptsConfiguredHTTPSHost(t *testing.T) {
	withResolver(t, "8.8.8.8")
	t.Setenv(allowedHostsEnv, "packages.example.test")
	if _, err := Validate(context.Background(), "https://packages.example.test/path/file.tgz"); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidatePanelDownload(t *testing.T) {
	const host = "172.16.1.3:8011"
	if _, err := ValidatePanelDownload("http://" + host + "/panel-api/v1/download/chart.tgz?download-ticket=ticket"); err != nil {
		t.Fatalf("ValidatePanelDownload() error = %v", err)
	}
	for _, raw := range []string{
		"http://" + host + "/other?download-ticket=ticket",
		"http://" + host + "/panel-api/v1/download/chart.tgz?download-ticket=ticket&other=value",
		"http://" + host + "/panel-api/v1/download/chart.tgz",
		"http:///panel-api/v1/download/chart.tgz?download-ticket=ticket",
	} {
		if _, err := ValidatePanelDownload(raw); err == nil {
			t.Fatalf("ValidatePanelDownload(%q) unexpectedly succeeded", raw)
		}
	}
}
