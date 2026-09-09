package types

import (
	"encoding/json"
	"testing"
)

func TestManifestVersionUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "string",
			data: `{"version":"1.0.0"}`,
			want: "1.0.0",
		},
		{
			name: "number",
			data: `{"version":1}`,
			want: "1",
		},
		{
			name: "object name",
			data: `{"version":{"id":10,"name":"2.0.0"}}`,
			want: "2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manifest Manifest
			if err := json.Unmarshal([]byte(tt.data), &manifest); err != nil {
				t.Fatal(err)
			}
			if got := manifest.Version.String(); got != tt.want {
				t.Fatalf("version = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManifestDomainRequirementsIgnoreModuleStartParams(t *testing.T) {
	tests := []struct {
		name        string
		application Application
		startParams []StartParams
		wantDomain  bool
		wantForce   bool
		wantHTTPS   bool
	}{
		{
			name: "unbound domain remains configurable",
			startParams: []StartParams{
				{ValuesText: "%DOMAIN_SSL_URL%", Required: true},
			},
			wantDomain: true,
			wantForce:  true,
			wantHTTPS:  true,
		},
		{
			name: "module domain is resolved from dependency",
			startParams: []StartParams{
				{ValuesText: "%DOMAIN_SSL_URL%", ModuleName: "test-environment", Required: true, Hidden: true},
			},
		},
		{
			name:        "console still requires domain",
			application: Application{FrontType: []string{"console"}},
			startParams: []StartParams{
				{ValuesText: "%DOMAIN_SSL_URL%", ModuleName: "test-environment", Required: true, Hidden: true},
			},
			wantDomain: true,
			wantForce:  true,
			wantHTTPS:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest := Manifest{
				Application: tt.application,
				Platform: Platform{Container: Container{
					StartParams: tt.startParams,
				}},
			}
			if got := manifest.RequireDomain(); got != tt.wantDomain {
				t.Fatalf("RequireDomain() = %v, want %v", got, tt.wantDomain)
			}
			if got := manifest.RequireDomainForce(); got != tt.wantForce {
				t.Fatalf("RequireDomainForce() = %v, want %v", got, tt.wantForce)
			}
			if got := manifest.RequireDomainHttps(); got != tt.wantHTTPS {
				t.Fatalf("RequireDomainHttps() = %v, want %v", got, tt.wantHTTPS)
			}
		})
	}
}
