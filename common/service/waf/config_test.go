package waf

import (
	"strings"
	"testing"
)

func TestCompileSelectsFilesAndInheritedRules(t *testing.T) {
	off := false
	name := SystemCatalog.Files[0].Name
	global := Policy{Categories: map[string]bool{name: false}, ParanoiaLevel: 3, EngineMode: "DetectionOnly", CustomRules: []Rule{{ID: 1000000, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000000,phase:1,deny"`}}}
	config, err := Compile(global, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, selected := range config["ruleFiles"].([]string) {
		if selected == name {
			t.Fatal("disabled file still loaded")
		}
	}
	if config["paranoiaLevel"] != 3 || config["engineMode"] != "DetectionOnly" {
		t.Fatal(config)
	}
	domain, err := Compile(global, &Policy{UseCRS: &off, Categories: map[string]bool{name: true}, DisabledGlobalIDs: []int{1000000}})
	if err != nil {
		t.Fatal(err)
	}
	if domain["useCRS"] != false || len(domain["secRules"].([]string)) != 0 {
		t.Fatal(domain)
	}
	found := false
	for _, selected := range domain["ruleFiles"].([]string) {
		if selected == name {
			found = true
		}
	}
	if !found {
		t.Fatal("domain did not restore file")
	}
}

func TestValidateRejectsInvalidPolicy(t *testing.T) {
	cases := []Policy{{Categories: map[string]bool{"../bad.conf": true}}, {EngineMode: "unknown"}, {ParanoiaLevel: 5}, {CustomRules: []Rule{{ID: 999999, Rule: `SecRule ARGS "@contains bad" "id:999999,deny"`}}}}
	for _, value := range cases {
		if _, err := Compile(value, nil); err == nil {
			t.Fatalf("accepted %+v", value)
		}
	}
	global := Policy{CustomRules: []Rule{{ID: 1000000, Rule: `SecRule REQUEST_URI "@streq /admin" "id:1000000,phase:1,deny"`}}}
	if _, err := Compile(global, &Policy{CustomRules: global.CustomRules}); err == nil {
		t.Fatal("domain rule accepted")
	}
	if _, err := Compile(global, &Policy{DisabledGlobalIDs: []int{1000001}}); err == nil {
		t.Fatal("unknown global ID accepted")
	}
	if _, err := Compile(global, &Policy{RequestBodyAccess: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	config, _ := Compile(global, &Policy{RequestBodyAccess: boolPtr(true)})
	if !strings.HasPrefix(strings.Join(config["secRules"].([]string), "\n"), "SecRequestBodyAccess On\n") {
		t.Fatal(config)
	}
}

func boolPtr(v bool) *bool { return &v }

func TestSystemRuleStates(t *testing.T) {
	name := validDetectionIDs[933100]
	global := Policy{RuleStates: map[int]bool{933100: false, 942100: false}, Categories: map[string]bool{name: false}}
	config, err := Compile(global, nil)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(config["secRules"].([]string), "\n")
	if strings.Contains(joined, "933100") || !strings.Contains(joined, "SecRuleRemoveById 942100") {
		t.Fatal(joined)
	}
	domain, err := Compile(global, &Policy{Categories: map[string]bool{name: true}, RuleStates: map[int]bool{942100: true}})
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(domain["secRules"].([]string), "\n")
	if !strings.Contains(joined, "SecRuleRemoveById 933100") || strings.Contains(joined, "942100") {
		t.Fatal(joined)
	}
	for _, id := range []int{0, 901001, 933011, 999999} {
		if _, err := Compile(Policy{RuleStates: map[int]bool{id: false}}, nil); err == nil {
			t.Fatalf("accepted ID %d", id)
		}
	}
}

func TestCatalogPreservesRuleDetails(t *testing.T) {
	for _, rule := range SystemCatalog.Rules {
		if rule.ID == 933100 {
			if rule.NameZh != "PHP 起始标签检测" || rule.ParanoiaLevel == nil || *rule.ParanoiaLevel != 1 {
				t.Fatal(rule)
			}
			return
		}
	}
	t.Fatal("rule 933100 missing")
}
