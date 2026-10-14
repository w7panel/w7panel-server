package panelauth

import (
	jwt "github.com/golang-jwt/jwt/v5"
	"testing"
	"time"
)

func TestCKMPanelScope(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test")
	p := Principal{Username: "alice", Actor: "admin", CKMUID: "uid-a", CVMName: "a", K3KNamespace: "k3k-alice", Role: "normal", PermissionName: "normal", TokenUse: TokenUseCKMPanel}
	raw, err := Issue(p, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(raw)
	if err != nil || got.Actor != "admin" || got.CKMUID != "uid-a" || got.CVMName != "a" {
		t.Fatalf("%#v/%v", got, err)
	}
	for _, change := range []func(*Principal){func(p *Principal) { p.Role = "founder" }, func(p *Principal) { p.PermissionName = "founder" }, func(p *Principal) { p.Actor = "" }, func(p *Principal) { p.CKMUID = "" }, func(p *Principal) { p.K3KNamespace = "k3k-bob" }} {
		bad := p
		change(&bad)
		if _, err := Issue(bad, time.Minute); err == nil {
			t.Fatal("issued invalid scope")
		}
	}
	claims := &Claims{}
	jwt.NewParser().ParseUnverified(raw, claims)
	claims.CVMName = "b"
	forged, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(signingKey())
	if _, err := Parse(forged); err == nil {
		t.Fatal("accepted inconsistent signed target and audience")
	}
	claims.CVMName = "a"
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(signingKey())
	if _, err := Parse(expired); err == nil {
		t.Fatal("accepted expired CKM session")
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
