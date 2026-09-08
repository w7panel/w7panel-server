package types

import (
	"encoding/json"
	"testing"
)

func TestManifestVersionUnmarshal(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "number", body: `{"version":2}`, want: "2"},
		{name: "string", body: `{"version":"2"}`, want: "2"},
		{name: "object", body: `{"version":{"zipfile":"da02328207ed87fd592a1c7b2a65405d","version":"2.9.14","createtime":"2026-06-04 18:24"}}`, want: "2.9.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manifest Manifest
			if err := json.Unmarshal([]byte(tt.body), &manifest); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if got := manifest.Version.String(); got != tt.want {
				t.Fatalf("manifest.Version.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
