package controller

import "testing"

func TestLocalRegistryPushRequest(t *testing.T) {
	req, err := localRegistryPushRequest("10.0.0.1:8000", "10.0.0.1:8000", "service-account-token")
	if err != nil {
		t.Fatal(err)
	}
	if req.RegisterDomain != "127.0.0.1:8000" || !req.RegistryPlainHTTP || req.RegistryUsername != "w7panel-k8s-token" || req.RegistryPassword != "service-account-token" {
		t.Fatalf("unexpected local registry push request: %+v", req)
	}
	for _, tc := range []struct{ domain, host, token string }{
		{"elsewhere.example:8000", "10.0.0.1:8000", "service-account-token"},
		{"10.0.0.1:8000", "10.0.0.1:8000", ""},
	} {
		if _, err := localRegistryPushRequest(tc.domain, tc.host, tc.token); err == nil {
			t.Fatalf("accepted invalid registry destination or token: domain=%q host=%q", tc.domain, tc.host)
		}
	}
}
