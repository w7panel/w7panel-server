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
}

func TestParseRejectsKubernetesLikeToken(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test-signing-key")
	if _, err := Parse("eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJzeXN0ZW06c2VydmljZWFjY291bnQ6ZGVmYXVsdDphZG1pbiJ9.signature"); err == nil {
		t.Fatal("Parse() accepted a Kubernetes-like token")
	}
}
