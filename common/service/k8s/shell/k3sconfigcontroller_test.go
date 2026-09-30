package shell

import "testing"

func TestSetK3sTlsSan(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty removes config", value: "", want: nil},
		{name: "filters empty entries", value: "10.0.0.1, example.com,", want: []string{"10.0.0.1", "example.com"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]interface{}{"tls-san": []string{"old.example.com"}}
			setK3sTlsSan(config, tt.value)
			got, ok := config["tls-san"].([]string)
			if tt.want == nil {
				if ok {
					t.Fatalf("tls-san = %v, want removed", got)
				}
				return
			}
			if !ok || len(got) != len(tt.want) {
				t.Fatalf("tls-san = %v, want %v", config["tls-san"], tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tls-san = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
