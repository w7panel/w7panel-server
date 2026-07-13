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
