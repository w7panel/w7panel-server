package console

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestParseLabels(t *testing.T) {
	labels, err := parseLabels([]string{"env=prod", "team=api", "empty="})
	if err != nil {
		t.Fatalf("parseLabels() error = %v", err)
	}

	want := map[string]string{"env": "prod", "team": "api", "empty": ""}
	for key, value := range want {
		if labels[key] != value {
			t.Errorf("labels[%q] = %q, want %q", key, labels[key], value)
		}
	}
}

func TestParseLabelsRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"missing-separator", "=missing-key"} {
		if _, err := parseLabels([]string{value}); err == nil {
			t.Errorf("parseLabels(%q) returned nil error", value)
		}
	}
}

func TestImageCommandRequiredFlags(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*cobra.Command)
		flags     []string
	}{
		{"tag", ImagesTag{}.Configure, []string{"source", "target"}},
		{"remove", ImagesRemove{}.Configure, []string{"target"}},
		{"label", ImagesLabel{}.Configure, []string{"name", "label"}},
		{"import", ImagesImport{}.Configure, []string{"name", "path"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			tt.configure(cmd)
			for _, name := range tt.flags {
				flag := cmd.Flags().Lookup(name)
				if flag == nil {
					t.Fatalf("flag %q was not registered", name)
				}
				required := flag.Annotations[cobra.BashCompOneRequiredFlag]
				if len(required) != 1 || required[0] != "true" {
					t.Errorf("flag %q is not required", name)
				}
			}
		})
	}
}
