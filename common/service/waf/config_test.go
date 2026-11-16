package waf

import (
	"strings"
	"testing"

	"github.com/corazawaf/coraza/v3"
)

func TestCompileInheritedPolicy(t *testing.T) {
	global := Policy{Categories: map[string]bool{"sqli": false}, CustomRules: []Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny"`}}}
	overrides := &Policy{Categories: map[string]bool{"sqli": true, "xss": false}, DisabledGlobalIDs: []int{1000001}}
	config, err := Compile(global, overrides, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules := config["secRules"].([]string)
	if len(rules) != 1 || rules[0] != "SecRuleRemoveById 941000-941999" {
		t.Fatalf("unexpected effective rules: %v", rules)
	}
}

func TestCompileUseCRSOverrideKeepsCustomRules(t *testing.T) {
	on, off := true, false
	global := Policy{UseCRS: &off, Categories: map[string]bool{"protocol": false}, CustomRules: []Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny"`}}}
	config, err := Compile(global, nil, nil)
	if err != nil || config["useCRS"] != false || len(config["secRules"].([]string)) != 2 {
		t.Fatalf("CRS off must retain custom rules: config=%v, err=%v", config, err)
	}
	if _, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives("SecRuleEngine On\n" + strings.Join(config["secRules"].([]string), "\n"))); err != nil {
		t.Fatalf("CRS off directives rejected: %v", err)
	}
	config, err = Compile(global, &Policy{UseCRS: &on}, nil)
	if err != nil || config["useCRS"] != true {
		t.Fatalf("domain CRS override failed: config=%v, err=%v", config, err)
	}
	config, err = Compile(Policy{UseCRS: &on}, &Policy{UseCRS: &off}, nil)
	if err != nil || config["useCRS"] != false {
		t.Fatalf("domain CRS disable failed: config=%v, err=%v", config, err)
	}
}

func TestRejectDuplicateRuleID(t *testing.T) {
	global := Policy{CustomRules: []Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /a" "id:1000001,phase:1,deny"`}}}
	overrides := &Policy{CustomRules: []Rule{{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /b" "id:1000001,phase:1,deny"`}}}
	if _, err := Compile(global, overrides, nil); err == nil {
		t.Fatal("duplicate rule ID accepted")
	}
}

func TestCompileVisualRuleForms(t *testing.T) {
	rules := []Rule{
		{ID: 1000001, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000001,phase:1,deny,status:403"`, Source: "visual"},
		{ID: 1000002, Rule: `SecRule REQUEST_URI "@contains /private" "id:1000002,phase:1,deny,status:403"`, Source: "visual"},
		{ID: 1000003, Rule: `SecRule REQUEST_METHOD "@streq POST" "id:1000003,phase:1,deny,status:403"`, Source: "visual"},
		{ID: 1000004, Rule: `SecRule REMOTE_ADDR "@ipMatch 192.0.2.0/24" "id:1000004,phase:1,deny,status:403"`, Source: "visual"},
		{ID: 1000005, Rule: `SecRule REQUEST_HEADERS:User-Agent "@contains badbot" "id:1000005,phase:1,deny,status:403"`, Source: "visual"},
	}
	if _, err := Compile(Policy{CustomRules: rules}, nil, nil); err != nil {
		t.Fatalf("visual rule syntax is invalid: %v", err)
	}
}

func TestCompileCanDisableInheritedLegacyRule(t *testing.T) {
	global := Policy{}
	overrides := &Policy{DisabledGlobalIDs: []int{101}}
	config, err := Compile(global, overrides, []string{`SecRule REQUEST_URI "@streq /a" "id:101,phase:1,deny"`, "SecRuleEngine On"})
	if err != nil {
		t.Fatal(err)
	}
	rules := config["secRules"].([]string)
	if len(rules) != 1 || rules[0] != "SecRuleEngine On" {
		t.Fatalf("unexpected rules: %v", rules)
	}
}

func TestCRSCategoryExclusionParsesInCoraza(t *testing.T) {
	config, err := Compile(Policy{Categories: map[string]bool{"xss": false}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	directives := `SecRuleEngine On
SecRule REQUEST_URI "@streq /blocked" "id:941001,phase:1,deny"` + "\n" + strings.Join(config["secRules"].([]string), "\n")
	if _, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives(directives)); err != nil {
		t.Fatalf("compiled CRS exclusion is invalid: %v", err)
	}
}

func TestSystemRuleOverridesAndCategoryPrecedence(t *testing.T) {
	global := Policy{SystemRules: map[string]bool{"942100": false, "941100": false}}
	overrides := &Policy{
		Categories:  map[string]bool{"xss": false},
		SystemRules: map[string]bool{"942100": true, "941100": false, "920100": false},
	}
	config, err := Compile(global, overrides, nil)
	if err != nil {
		t.Fatal(err)
	}
	rules := config["secRules"].([]string)
	want := []string{"SecRuleRemoveById 941000-941999", "SecRuleRemoveById 920100"}
	if len(rules) != len(want) {
		t.Fatalf("unexpected rules: %v", rules)
	}
	for index := range want {
		if rules[index] != want[index] {
			t.Fatalf("unexpected rules: %v", rules)
		}
	}
}

func TestRejectUnknownSystemRuleID(t *testing.T) {
	for _, id := range []string{"foo", "9200000", "910001", "920999", "0920100"} {
		if err := Validate(Policy{SystemRules: map[string]bool{id: false}}, nil); err == nil {
			t.Fatalf("invalid system rule ID %q accepted", id)
		}
	}
}

func TestCompileVisualChainedRule(t *testing.T) {
	rule := "SecRule REQUEST_METHOD \"@streq POST\" \"id:1000003,phase:2,deny,status:403,chain\"\nSecRule ARGS \"@contains attack\" \"t:none\""
	if _, err := Compile(Policy{CustomRules: []Rule{{ID: 1000003, Rule: rule, Source: "visual"}}}, nil, nil); err != nil {
		t.Fatalf("chained rule rejected: %v", err)
	}
	injected := rule + "\nSecRuleEngine Off"
	if _, err := Compile(Policy{CustomRules: []Rule{{ID: 1000003, Rule: injected}}}, nil, nil); err == nil {
		t.Fatal("non-SecRule directive accepted")
	}
}

func TestCRSIndividualExclusionParsesInCoraza(t *testing.T) {
	config, err := Compile(Policy{SystemRules: map[string]bool{"942100": false}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	directives := `SecRuleEngine On
SecRule REQUEST_URI "@streq /blocked" "id:942100,phase:1,deny"` + "\n" + strings.Join(config["secRules"].([]string), "\n")
	if _, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives(directives)); err != nil {
		t.Fatalf("individual CRS exclusion is invalid: %v", err)
	}
}

func TestChainedRuleRequiresBothConditions(t *testing.T) {
	rule := "SecRule REQUEST_URI \"@streq /admin\" \"id:1000004,phase:1,deny,status:403,chain\"\nSecRule REQUEST_METHOD \"@streq POST\" \"t:none\""
	config, err := Compile(Policy{CustomRules: []Rule{{ID: 1000004, Rule: rule}}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	waf, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives("SecRuleEngine On\n" + strings.Join(config["secRules"].([]string), "\n")))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST"} {
		tx := waf.NewTransaction()
		tx.ProcessURI("/admin", method, "HTTP/1.1")
		blocked := tx.ProcessRequestHeaders() != nil
		tx.Close()
		if blocked != (method == "POST") {
			t.Fatalf("method %s: blocked=%v", method, blocked)
		}
	}
}
