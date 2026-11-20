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
