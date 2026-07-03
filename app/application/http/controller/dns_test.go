package controller

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/coredns"
)

func TestK3kDNSServerStatusDisabled(t *testing.T) {
	status := k3kDNSServerStatus()
	if status.Enabled {
		t.Fatal("expected k3k dns server status to be disabled")
	}
	if status.ServiceName != coredns.PublicDNSServiceName {
		t.Fatalf("service name = %q, want %q", status.ServiceName, coredns.PublicDNSServiceName)
	}
	if status.ServiceType != "" {
		t.Fatalf("service type = %q, want empty", status.ServiceType)
	}
	if len(status.ExternalIPs) != 0 {
		t.Fatalf("external ips = %v, want empty", status.ExternalIPs)
	}
}

func TestK3kDNSServerUpdateUnsupportedError(t *testing.T) {
	if errK3kDNSServerUnsupported == nil {
		t.Fatal("expected k3k dns server update error")
	}
	if errK3kDNSServerUnsupported.Error() != "private dns server is not supported in k3k cluster" {
		t.Fatalf("unexpected error: %v", errK3kDNSServerUnsupported)
	}
}
