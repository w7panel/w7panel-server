package panelauth

import (
	jwt "github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestRejectRetiredSessionToken(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test")
	if _, err := Issue(Principal{Username: "alice", TokenUse: "ckm-panel"}, time.Minute); err == nil {
		t.Fatal("issued retired session token")
	}
	raw, err := Issue(Principal{Username: "alice", TokenUse: TokenUsePanel}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims := &Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(raw, claims); err != nil {
		t.Fatal(err)
	}
	claims.TokenUse = "ckm-panel"
	raw, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(signingKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(raw); err == nil {
		t.Fatal("accepted retired session token")
	}
}

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
	got := audience(Principal{Username: "alice", Role: "normal", ConsoleID: "console-1"})
	want := []string{"alice", "normal", "console-1", "", "", "https://kubernetes.default.svc.cluster.local", "k3s"}
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
