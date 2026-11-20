package waf

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/corazawaf/coraza/v3"
)

type Rule struct {
	ID     int    `json:"id"`
	Rule   string `json:"rule"`
	Source string `json:"source,omitempty"`
}

type Policy struct {
	UseCRS            *bool           `json:"useCRS,omitempty"`
	RequestBodyAccess *bool           `json:"requestBodyAccess,omitempty"`
	Categories        map[string]bool `json:"categories"`
	CustomRules       []Rule          `json:"customRules"`
	DisabledGlobalIDs []int           `json:"disabledGlobalIds,omitempty"`
	ParanoiaLevel     int             `json:"paranoiaLevel,omitempty"`
	EngineMode        string          `json:"engineMode,omitempty"`
}

var ruleID = regexp.MustCompile(`(?i)\bid\s*:\s*(\d+)`)

func Validate(global Policy, overrides *Policy) error {
	for _, policy := range []*Policy{&global, overrides} {
		if policy == nil {
			continue
		}
		for name := range policy.Categories {
			if !validRuleFiles[name] {
				return fmt.Errorf("unknown CRS rule file %q", name)
			}
		}
	}
	if global.EngineMode != "" && global.EngineMode != "On" && global.EngineMode != "Off" && global.EngineMode != "DetectionOnly" {
		return errors.New("invalid engine mode")
	}
	if global.ParanoiaLevel != 0 && (global.ParanoiaLevel < 1 || global.ParanoiaLevel > 4) {
		return errors.New("paranoia level must be 1-4")
	}
	if overrides != nil {
		if len(overrides.CustomRules) > 0 || overrides.EngineMode != "" || overrides.ParanoiaLevel != 0 {
			return errors.New("domain may only inherit or disable global rules")
		}
		known := make(map[int]bool)
		for _, rule := range global.CustomRules {
			known[rule.ID] = true
		}
		for _, id := range overrides.DisabledGlobalIDs {
			if !known[id] {
				return fmt.Errorf("unknown global custom rule ID %d", id)
			}
		}
	}
	if len(global.CustomRules) > 100 {
		return errors.New("at most 100 custom rules are allowed")
	}
	seen := make(map[int]bool)
	for _, rule := range global.CustomRules {
		value := strings.TrimSpace(rule.Rule)
		if len(value) == 0 || len(value) > 8192 || strings.Contains(value, "\r") {
			return errors.New("custom rule must contain SecRule directives of at most 8192 bytes")
		}
		for _, line := range strings.Split(value, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "SecRule ") {
				return errors.New("custom rule may only contain SecRule directives")
			}
		}
		matches := ruleID.FindAllStringSubmatch(value, -1)
		if len(matches) != 1 || fmt.Sprint(rule.ID) != matches[0][1] || rule.ID < 1000000 || seen[rule.ID] {
			return fmt.Errorf("invalid or duplicate custom rule ID %d", rule.ID)
		}
		if _, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives("SecRuleEngine On\n" + value)); err != nil {
			return fmt.Errorf("invalid custom rule %d: %w", rule.ID, err)
		}
		seen[rule.ID] = true
	}
	return nil
}

func Compile(global Policy, overrides *Policy) (map[string]interface{}, error) {
	if err := Validate(global, overrides); err != nil {
		return nil, err
	}
	useCRS := true
	if global.UseCRS != nil {
		useCRS = *global.UseCRS
	}
	if overrides != nil && overrides.UseCRS != nil {
		useCRS = *overrides.UseCRS
	}
	active := make(map[string]bool, len(validRuleFiles))
	for name := range validRuleFiles {
		enabled, ok := global.Categories[name]
		active[name] = !ok || enabled
	}
	if overrides != nil {
		for name, enabled := range overrides.Categories {
			active[name] = enabled
		}
	}
	files := make([]string, 0, len(active))
	for name, enabled := range active {
		if enabled {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	disabled := make(map[int]bool)
	if overrides != nil {
		for _, id := range overrides.DisabledGlobalIDs {
			disabled[id] = true
		}
	}
	secRules := []string{}
	requestBodyAccess := global.RequestBodyAccess != nil && *global.RequestBodyAccess
	if overrides != nil && overrides.RequestBodyAccess != nil {
		requestBodyAccess = *overrides.RequestBodyAccess
	}
	if requestBodyAccess {
		secRules = append(secRules, "SecRequestBodyAccess On")
	}
	for _, rule := range global.CustomRules {
		if !disabled[rule.ID] {
			secRules = append(secRules, strings.TrimSpace(rule.Rule))
		}
	}
	mode := global.EngineMode
	if mode == "" {
		mode = "On"
	}
	level := global.ParanoiaLevel
	if level == 0 {
		level = 1
	}
	return map[string]interface{}{"useCRS": useCRS, "ruleFiles": files, "secRules": secRules, "engineMode": mode, "paranoiaLevel": level}, nil
}
