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
