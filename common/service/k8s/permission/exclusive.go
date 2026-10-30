package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ExclusiveConfigMapName = "w7panel-permission-exclusives"
const MainClusterType = "main"
const ChildClusterType = "child"

func CurrentClusterType() string {
	if helper.IsChildAgent() {
		return ChildClusterType
	}
	return MainClusterType
}

// ExclusiveRule uses Permission menu paths and API rules; menuKeys covers old stored key values.
type ExclusiveRule struct {
	MenuRule     string                             `json:"menuRule"`
	MenuKeys     []string                           `json:"menuKeys,omitempty"`
	ClusterTypes []string                           `json:"clusterTypes"`
	APIRules     []configv1alpha1.PermissionAPIRule `json:"apiRules,omitempty"`
}

type ExclusivePolicy struct {
	Rules []ExclusiveRule `json:"rules"`
}

func LoadExclusivePolicy(ctx context.Context, sdk *k8s.Sdk) (ExclusivePolicy, error) {
	if helper.IsChildAgent() {
		remote, err := loadMainPanelPolicy(ctx)
		if err != nil {
			return ExclusivePolicy{}, err
		}
		return ParseExclusivePolicy(string(remote))
	}
	cm, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(ctx, ExclusiveConfigMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ExclusivePolicy{Rules: []ExclusiveRule{}}, nil
	}
	if err != nil {
		return ExclusivePolicy{}, err
	}
	return ParseExclusivePolicy(cm.Data["policy"])
}

func ParseExclusivePolicy(raw string) (ExclusivePolicy, error) {
	if raw == "" {
		return ExclusivePolicy{}, fmt.Errorf("专属菜单策略缺少 policy")
	}
	var policy ExclusivePolicy
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		return ExclusivePolicy{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ExclusivePolicy{}, fmt.Errorf("专属菜单策略包含多余内容")
	}
	if policy.Rules == nil {
		return ExclusivePolicy{}, fmt.Errorf("专属菜单策略缺少 rules")
	}
	seen := map[string]bool{}
	seenKeys := map[string]bool{}
	seenAPI := []string{}
	for _, rule := range policy.Rules {
		if rule.MenuRule == "" || strings.Trim(rule.MenuRule, "/ ") != rule.MenuRule || strings.Contains(rule.MenuRule, "//") || strings.Contains(rule.MenuRule, "*") || seen[rule.MenuRule] || len(rule.ClusterTypes) == 0 {
			return ExclusivePolicy{}, fmt.Errorf("无效或重复的专属菜单规则 %q", rule.MenuRule)
		}
		seen[rule.MenuRule] = true
		for _, key := range rule.MenuKeys {
			if key == "" || seenKeys[key] {
				return ExclusivePolicy{}, fmt.Errorf("无效或重复的专属菜单键 %q", key)
			}
			seenKeys[key] = true
		}
		for _, clusterType := range rule.ClusterTypes {
			if clusterType != MainClusterType && clusterType != ChildClusterType {
				return ExclusivePolicy{}, fmt.Errorf("专属菜单 %q 包含无效集群类型", rule.MenuRule)
			}
		}
		for _, api := range rule.APIRules {
			if api.Path != "*" && (!strings.HasPrefix(api.Path, "/") || strings.Contains(api.Path, "//") || strings.TrimSpace(api.Path) != api.Path) || len(api.Method) == 0 {
				return ExclusivePolicy{}, fmt.Errorf("专属菜单 %q 的 API 规则无效", rule.MenuRule)
			}
			for _, previous := range seenAPI {
				if apiPatternsOverlap(previous, api.Path) {
					return ExclusivePolicy{}, fmt.Errorf("重叠的专属 API 路径 %q 与 %q", previous, api.Path)
				}
			}
			seenAPI = append(seenAPI, api.Path)
			seenMethods := map[string]bool{}
			for _, verb := range api.Method {
				method := httpMethodForVerb(verb)
				if method == "" || seenMethods[method] {
					return ExclusivePolicy{}, fmt.Errorf("专属菜单 %q 的 API 方法无效", rule.MenuRule)
				}
				seenMethods[method] = true
			}
		}
	}
	return policy, nil
}

func (p ExclusivePolicy) AllowsMenu(clusterType, menu string) bool {
	menu = strings.Trim(menu, "/")
	for _, rule := range p.Rules {
		if menu == rule.MenuRule || containsExclusiveValue(rule.MenuKeys, menu) || menu == "*" || strings.HasSuffix(menu, "/*") && strings.HasPrefix(rule.MenuRule, strings.TrimSuffix(menu, "/*")+"/") {
			if !containsExclusiveValue(rule.ClusterTypes, clusterType) {
				return false
			}
		}
	}
	return true
}

func (p ExclusivePolicy) AllowsAPI(clusterType, method, path string) bool {
	for _, rule := range p.Rules {
		if MatchAPI(APIRulesToMap(rule.APIRules), method, path) && !containsExclusiveValue(rule.ClusterTypes, clusterType) {
			return false
		}
	}
	return true
}

func (p ExclusivePolicy) ValidateGrant(clusterType string, menus []string, apis []configv1alpha1.PermissionAPIRule) error {
	for _, rule := range p.Rules {
		if containsExclusiveValue(rule.ClusterTypes, clusterType) {
			continue
		}
		for _, selected := range menus {
			selected = strings.Trim(selected, "/")
			if selected == "*" || selected == rule.MenuRule || containsExclusiveValue(rule.MenuKeys, selected) || strings.HasSuffix(selected, "/*") && strings.HasPrefix(rule.MenuRule, strings.TrimSuffix(selected, "/*")+"/") {
				return fmt.Errorf("%s 集群不允许授予菜单 %q", clusterType, rule.MenuRule)
			}
		}
		for _, api := range rule.APIRules {
			for _, verb := range api.Method {
				for _, selected := range apis {
					if !apiPatternsOverlap(selected.Path, api.Path) {
						continue
					}
					for _, selectedVerb := range selected.Method {
						if selectedVerb == "*" || httpMethodForVerb(selectedVerb) == httpMethodForVerb(verb) {
							return fmt.Errorf("%s 集群不允许授予 API %s", clusterType, api.Path)
						}
					}
				}
			}
		}
	}
	return nil
}

// apiPatternsOverlap treats '*' as the same glob used by Permission API paths.
func apiPatternsOverlap(a, b string) bool {
	seen := map[[2]int]bool{}
	var visit func(int, int) bool
	visit = func(i, j int) bool {
		state := [2]int{i, j}
		if seen[state] {
			return false
		}
		seen[state] = true
		if i == len(a) && j == len(b) {
			return true
		}
		if i < len(a) && a[i] == '*' && visit(i+1, j) {
			return true
		}
		if j < len(b) && b[j] == '*' && visit(i, j+1) {
			return true
		}
		if i == len(a) || j == len(b) {
			return false
		}
		if a[i] == '*' {
			return visit(i, j+1)
		}
		if b[j] == '*' {
			return visit(i+1, j)
		}
		return a[i] == b[j] && visit(i+1, j+1)
	}
	return visit(0, 0)
}

func (p ExclusivePolicy) FilterMenus(clusterType string, menus []string) []string {
	result := make([]string, 0, len(menus))
	for _, menu := range menus {
		if p.AllowsMenu(clusterType, menu) {
			result = append(result, menu)
		}
	}
	return result
}

func httpMethodForVerb(verb string) string {
	switch strings.ToLower(verb) {
	case "get", "list":
		return "GET"
	case "create":
		return "POST"
	case "update":
		return "PUT"
	case "patch":
		return "PATCH"
	case "delete":
		return "DELETE"
	default:
		return ""
	}
}

func containsExclusiveValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
