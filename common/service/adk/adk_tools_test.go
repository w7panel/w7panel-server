package adk

import "testing"

func TestKubectlTool(t *testing.T) {
	tool, err := kubectlTool()
	if err != nil {
		t.Fatal(err)
	}
	if tool.Name() != "kubectl" {
		t.Fatalf("tool name = %q", tool.Name())
	}
}

func TestKubectlArgs(t *testing.T) {
	for _, test := range []struct {
		command string
		valid   bool
	}{
		{"kubectl get pods", true},
		{"kubectl get secrets", false},
		{"kubectl --token=x get pods", false},
		{"kubectl get pods; whoami", false},
		{"echo pods", false},
	} {
		_, err := kubectlArgs(test.command)
		if (err == nil) != test.valid {
			t.Errorf("kubectlArgs(%q) error = %v", test.command, err)
		}
	}
}
