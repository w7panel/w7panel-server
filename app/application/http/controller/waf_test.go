package controller

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/waf"
)

func TestWAFPolicyRoundTripKeepsVisualRuleSource(t *testing.T) {
	want := waf.Policy{SystemRules: map[string]bool{"942100": false}, CustomRules: []waf.Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny,status:403"`, Source: "visual"}}}
	got := policyFrom(policyMap(want))
	if len(got.CustomRules) != 1 || got.CustomRules[0] != want.CustomRules[0] {
		t.Fatalf("WAF policy round-trip lost visual rule source: %+v", got.CustomRules)
	}
	if got.SystemRules["942100"] != false || len(got.SystemRules) != 1 {
		t.Fatalf("WAF policy round-trip lost system rule override: %+v", got.SystemRules)
	}
}

func TestParseWAFLogsFiltersExactDomainAndStripsQuery(t *testing.T) {
	data := []byte(`{"authority":"example.com","path":"/login?token=secret","response_code":403,"response_code_details":"waf","request_id":"12345678-1234-1234-1234-123456789012","start_time":"2026-11-12T00:00:00Z","x_forwarded_for":"192.0.2.1, 10.0.0.1"}` + "\n" +
		`{"authority":"other.example.com","path":"/","response_code_details":"waf","request_id":"other"}` + "\n" +
		`{"authority":"example.com","path":"/normal","response_code_details":"via_upstream","request_id":"normal"}` + "\n")
	logs := parseWAFLogs(data, "example.com")
	if len(logs) != 1 || logs[0].Path != "/login" || logs[0].ClientIP != "192.0.2.1" {
		t.Fatalf("unexpected WAF logs: %+v", logs)
	}
}
