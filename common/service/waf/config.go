package waf

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/corazawaf/coraza/v3"
)

// Categories are the user-facing OWASP CRS 3.3.2 groups. CRS setup and
// blocking evaluation rules remain loaded even when a group is disabled.
var Categories = map[string]string{
	"method": "911000-911999", "scanner": "913000-913999",
	"protocol": "920000-922999", "lfi": "930000-930999",
	"rfi": "931000-931999", "rce": "932000-932999",
	"php": "933000-933999", "generic": "934000-934999",
	"xss": "941000-941999", "sqli": "942000-942999",
	"session": "943000-943999", "java": "944000-944999",
	"leak": "950000-955999",
}

type Rule struct {
	ID     int    `json:"id"`
	Rule   string `json:"rule"`
	Source string `json:"source,omitempty"`
}

type Policy struct {
	Categories        map[string]bool `json:"categories"`
	SystemRules       map[string]bool `json:"systemRules,omitempty"`
	CustomRules       []Rule          `json:"customRules"`
	DisabledGlobalIDs []int           `json:"disabledGlobalIds,omitempty"`
	LegacyRules       []string        `json:"legacyRules,omitempty"`
}

var ruleID = regexp.MustCompile(`(?i)\bid\s*:\s*(\d+)`)

func Validate(global Policy, overrides *Policy) error {
	for _, policy := range []*Policy{&global, overrides} {
		if policy == nil {
			continue
		}
		for name := range policy.Categories {
			if _, ok := Categories[name]; !ok {
				return fmt.Errorf("unknown CRS category %q", name)
			}
		}
		for value := range policy.SystemRules {
			id, err := strconv.Atoi(value)
			if err != nil || strconv.Itoa(id) != value || !systemRuleIDs[id] {
				return fmt.Errorf("unknown CRS rule ID %q", value)
			}
		}
	}
	seen := make(map[int]bool)
	allRules := append(append([]Rule{}, global.CustomRules...), rulesOf(overrides)...)
	if len(allRules) > 100 {
		return errors.New("at most 100 custom rules are allowed")
	}
	for _, rule := range allRules {
		text := strings.TrimSpace(rule.Rule)
		if len(text) > 8192 || strings.Contains(text, "\r") {
			return errors.New("custom rule must contain SecRule directives of at most 8192 bytes")
		}
		for _, line := range strings.Split(text, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "SecRule ") {
				return errors.New("custom rule may only contain SecRule directives")
			}
		}
		matches := ruleID.FindAllStringSubmatch(text, -1)
		if len(matches) != 1 || fmt.Sprint(rule.ID) != matches[0][1] || rule.ID < 1000000 || seen[rule.ID] {
			return fmt.Errorf("invalid or duplicate custom rule ID %d", rule.ID)
		}
		if _, err := coraza.NewWAF(coraza.NewWAFConfig().WithDirectives("SecRuleEngine On\n" + text)); err != nil {
			return fmt.Errorf("invalid custom rule %d: %w", rule.ID, err)
		}
		seen[rule.ID] = true
	}
	return nil
}

func systemRuleCategory(id int) string {
	for name, value := range Categories {
		var start, end int
		if _, err := fmt.Sscanf(value, "%d-%d", &start, &end); err == nil && id >= start && id <= end {
			return name
		}
	}
	return ""
}

func rulesOf(policy *Policy) []Rule {
	if policy == nil {
		return nil
	}
	return policy.CustomRules
}

// Compile returns the existing Higress WAF wire format. The w7 fields are
// stored alongside it so domain overrides survive future global changes.
func Compile(global Policy, overrides *Policy, legacy []string) (map[string]interface{}, error) {
	if err := Validate(global, overrides); err != nil {
		return nil, err
	}
	for _, old := range legacy {
		match := ruleID.FindStringSubmatch(old)
		if len(match) != 2 {
			continue
		}
		for _, rule := range append(append([]Rule{}, global.CustomRules...), rulesOf(overrides)...) {
			if fmt.Sprint(rule.ID) == match[1] {
				return nil, fmt.Errorf("custom rule ID %d conflicts with a legacy rule", rule.ID)
			}
		}
	}
	active := make(map[string]bool, len(global.Categories))
	for id, value := range global.Categories {
		active[id] = value
	}
	if overrides != nil {
		for id, value := range overrides.Categories {
			active[id] = value
		}
	}
	systemRules := make(map[string]bool, len(global.SystemRules))
	for id, value := range global.SystemRules {
		systemRules[id] = value
	}
	if overrides != nil {
		for id, value := range overrides.SystemRules {
			systemRules[id] = value
		}
	}
	ids := make([]string, 0, len(Categories))
	for id := range Categories {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	secRules := make([]string, 0, len(legacy)+len(Categories)+len(global.CustomRules)+len(rulesOf(overrides)))
	disabled := make(map[int]bool)
	if overrides != nil {
		for _, id := range overrides.DisabledGlobalIDs {
			disabled[id] = true
		}
	}
	for _, rule := range legacy {
		match := ruleID.FindStringSubmatch(rule)
		if len(match) == 2 {
			var id int
			_, _ = fmt.Sscan(match[1], &id)
			if disabled[id] {
				continue
			}
		}
		secRules = append(secRules, rule)
	}
	for _, id := range ids {
		if value, ok := active[id]; ok && !value {
			secRules = append(secRules, "SecRuleRemoveById "+Categories[id])
		}
	}
	disabledSystemIDs := make([]int, 0, len(systemRules))
	for value, enabled := range systemRules {
		id, _ := strconv.Atoi(value)
		if enabled {
			continue
		}
		if categoryEnabled, ok := active[systemRuleCategory(id)]; ok && !categoryEnabled {
			continue
		}
		disabledSystemIDs = append(disabledSystemIDs, id)
	}
	sort.Ints(disabledSystemIDs)
	for _, id := range disabledSystemIDs {
		secRules = append(secRules, fmt.Sprintf("SecRuleRemoveById %d", id))
	}
	for _, rule := range global.CustomRules {
		if !disabled[rule.ID] {
			secRules = append(secRules, strings.TrimSpace(rule.Rule))
		}
	}
	for _, rule := range rulesOf(overrides) {
		secRules = append(secRules, strings.TrimSpace(rule.Rule))
	}
	return map[string]interface{}{"useCRS": true, "secRules": secRules}, nil
}
