package dnsprofile

import "testing"

func TestProviderCredentials(t *testing.T) {
	for _, provider := range []string{"alidns", "dnspod", "cloudflare", "route53"} {
		if _, err := ProviderCredentials(provider, nil); err == nil {
			t.Fatalf("%s accepted missing credentials", provider)
		}
	}
	values := map[string]string{"ALICLOUD_ACCESS_KEY": "id", "ALICLOUD_SECRET_KEY": "secret", "unused": "must-not-be-stored"}
	got, err := ProviderCredentials("alidns", values)
	if err != nil || len(got) != 2 || got["unused"] != "" {
		t.Fatalf("unexpected credential selection: %v, %v", got, err)
	}
}

func TestValidZone(t *testing.T) {
	for _, zone := range []string{"example.com", "sub.example.com"} {
		if !validZone(zone) { t.Fatalf("valid zone rejected: %s", zone) }
	}
	for _, zone := range []string{"*.example.com", "-example.com", "example.com/other", "localhost", ""} {
		if validZone(zone) { t.Fatalf("invalid zone accepted: %s", zone) }
	}
}
