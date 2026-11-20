package controller

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/waf"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWAFPolicyRoundTripKeepsVisualRuleSource(t *testing.T) {
	off := false
	want := waf.Policy{UseCRS: &off, CustomRules: []waf.Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny,status:403"`, Source: "visual"}}}
	old := policyMap(want)
	old["systemRules"] = map[string]bool{"942100": false}
	got := policyFrom(old)
	if len(got.CustomRules) != 1 || got.CustomRules[0] != want.CustomRules[0] {
		t.Fatalf("WAF policy round-trip lost visual rule source: %+v", got.CustomRules)
	}
	if _, exists := policyMap(got)["systemRules"]; exists {
		t.Fatal("old single-rule setting survived policy round-trip")
	}
	if got.UseCRS == nil || *got.UseCRS {
		t.Fatalf("WAF policy round-trip lost CRS switch: %+v", got.UseCRS)
	}
}

func TestWAFConfigReadsExistingCRSSetting(t *testing.T) {
	plugin := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"defaultConfig": map[string]interface{}{"useCRS": false}}}}
	policy, _, err := wafConfig(plugin)
	if err != nil || policy.UseCRS == nil || *policy.UseCRS {
		t.Fatalf("existing CRS setting lost: policy=%+v, err=%v", policy, err)
	}
}
