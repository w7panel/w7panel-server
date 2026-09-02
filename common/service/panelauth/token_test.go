package panelauth

import (
	"testing"
	"time"
)

func TestIssueAndParse(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test-signing-key")
	raw, err := Issue(Principal{
		Username:       "alice",
		PermissionName: "normal",
		Role:           "normal",
		TokenUse:       TokenUsePanel,
	}, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	principal, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if principal.Username != "alice" || principal.PermissionName != "normal" || principal.TokenUse != TokenUsePanel {
		t.Fatalf("Parse() principal = %#v", principal)
	}
	if got := audience(*principal); len(got) != 7 || got[0] != "alice" || got[5] != "https://kubernetes.default.svc.cluster.local" || got[6] != "k3s" {
		t.Fatalf("audience = %#v", got)
	}
}

func TestAudienceMatchesDevV1Shape(t *testing.T) {
	got := audience(Principal{Username: "alice", Role: "normal", ConsoleID: "console-1", CVMName: "ckm-1", K3KNamespace: "ns-1"})
	want := []string{"alice", "normal", "console-1", "ckm-1", "ns-1", "https://kubernetes.default.svc.cluster.local", "k3s"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("audience[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseRejectsKubernetesLikeToken(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test-signing-key")
	if _, err := Parse("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDphZG1pbiJ9.signature"); err == nil {
		t.Fatal("Parse() accepted a Kubernetes-like token")
	}
}
