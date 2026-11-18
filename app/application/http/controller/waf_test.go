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
	policy, _, _, err := wafConfig(plugin)
	if err != nil || policy.UseCRS == nil || *policy.UseCRS {
		t.Fatalf("existing CRS setting lost: policy=%+v, err=%v", policy, err)
	}
}

func TestParseWAFLogsFiltersExactDomainAndStripsQuery(t *testing.T) {
	data := []byte(`{"authority":"example.com","path":"/login?token=secret","response_code":403,"response_code_details":"waf","request_id":"12345678-1234-1234-1234-123456789012","start_time":"2026-11-12T00:00:00Z","x_forwarded_for":"192.0.2.1, 10.0.0.1"}` + "\n" +
		`{"authority":"example.com","path":"/waf-e2e-20261118","response_code":"403","response_code_details":"via_wasm::higress-system.w7panel-pluginwaf::waf","request_id":"9f41bf39-f432-4dd7-acef-f0fc8ff095e4"}` + "\n" +
		`{"authority":"other.example.com","path":"/","response_code_details":"waf","request_id":"other"}` + "\n" +
		`{"authority":"example.com","path":"/other-plugin","response_code_details":"via_wasm::higress-system.other-plugin::waf","request_id":"other-plugin"}` + "\n" +
		`{"authority":"example.com","path":"/other-reason","response_code_details":"via_wasm::higress-system.w7panel-pluginwaf::other","request_id":"other-reason"}` + "\n" +
		`{"authority":"example.com","path":"/normal","response_code_details":"via_upstream","request_id":"normal"}` + "\n")
	logs := parseWAFLogs(data, "example.com")
	if len(logs) != 2 || logs[0].Path != "/login" || logs[0].ClientIP != "192.0.2.1" || logs[1].Key != "9f41bf39-f432-4dd7-acef-f0fc8ff095e4" {
		t.Fatalf("unexpected WAF logs: %+v", logs)
	}
}
