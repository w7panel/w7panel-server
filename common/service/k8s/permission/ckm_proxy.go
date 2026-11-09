package permission

import (
	"net/http"
	"strconv"
	"strings"

	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// AuthorizeCKMK8sProxy checks the selected main Permission against the
// Kubernetes resource addressed by an embedded panel request.
func AuthorizeCKMK8sProxy(p *configv1alpha1.Permission, req *http.Request) bool {
	if p == nil || req == nil || req.URL == nil || !strings.HasPrefix(req.URL.Path, "/k8s-proxy/") {
		return false
	}
	path := strings.TrimPrefix(req.URL.Path, "/k8s-proxy")
	if strings.Contains(strings.ToLower(req.URL.EscapedPath()), "%2f") || strings.Contains(path, "\\") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	group := ""
	var resourcePath []string
	switch {
	case len(segments) >= 3 && segments[0] == "api":
		resourcePath = segments[2:]
	case len(segments) >= 4 && segments[0] == "apis":
		group = segments[1]
		resourcePath = segments[3:]
	default:
		return matchCKMNonResource(p.Spec.RBACRules, req.Method, path)
	}
	if len(resourcePath) >= 3 && resourcePath[0] == "namespaces" {
		resourcePath = resourcePath[2:]
	}
	if len(resourcePath) < 1 || len(resourcePath) > 3 {
		return false
	}
	resource, name := resourcePath[0], ""
	if len(resourcePath) >= 2 {
		name = resourcePath[1]
	}
	if len(resourcePath) == 3 {
		resource += "/" + resourcePath[2]
	}
	verb := ""
	switch req.Method {
	case http.MethodGet:
		verb = "get"
		if name == "" {
			verb = "list"
		}
		if watch, _ := strconv.ParseBool(req.URL.Query().Get("watch")); watch {
			verb = "watch"
		}
	case http.MethodPost:
		verb = "create"
	case http.MethodPut:
		verb = "update"
	case http.MethodPatch:
		verb = "patch"
	case http.MethodDelete:
		verb = "delete"
		if name == "" {
			verb = "deletecollection"
		}
	default:
		return false
	}
	for _, rule := range p.Spec.RBACRules {
		if ckmRuleContains(rule.APIGroups, group) && ckmRuleContains(rule.Resources, resource) && ckmRuleContains(rule.Verbs, verb) && (len(rule.ResourceNames) == 0 || name != "" && ckmRuleContains(rule.ResourceNames, name)) {
			return true
		}
	}
	return false
}

func matchCKMNonResource(rules []rbacv1.PolicyRule, method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	for _, rule := range rules {
		if !ckmRuleContains(rule.Verbs, "get") {
			continue
		}
		for _, allowed := range rule.NonResourceURLs {
			if allowed == path || strings.HasSuffix(allowed, "*") && strings.HasPrefix(path, strings.TrimSuffix(allowed, "*")) {
				return true
			}
		}
	}
	return false
}

func ckmRuleContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == "*" || candidate == value {
			return true
		}
	}
	return false
}
