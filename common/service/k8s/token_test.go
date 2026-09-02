package k8s

import "testing"

func TestK8sTokenK3KAudienceShapeMatchesLoginToken(t *testing.T) {
	newToken := func(audiences ...string) *K8sToken {
		aud := make([]interface{}, len(audiences))
		for i, audience := range audiences {
			aud[i] = audience
		}
		return &K8sToken{
			claims:       map[string]interface{}{"aud": aud},
			claimsParsed: true,
		}
	}

	token := newToken("user", "founder", "console", "ckm-1", "namespace", "api-server", "k3s")
	if !token.IsK3kCluster() {
		t.Fatal("seven-item K3K audience must be recognized")
	}
	if got := token.GetCvmName(); got != "ckm-1" {
		t.Fatalf("CVM name = %q, want %q", got, "ckm-1")
	}

	extraAudienceToken := newToken("user", "founder", "console", "ckm-1", "namespace", "api-server", "k3s", "policy")
	if extraAudienceToken.IsK3kCluster() {
		t.Fatal("audience shape that differs from login-issued K3K tokens must not be recognized")
	}
	if got := extraAudienceToken.GetCvmName(); got != "" {
		t.Fatalf("CVM name for non-login audience = %q, want empty", got)
	}
}
