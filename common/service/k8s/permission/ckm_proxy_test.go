package permission

import (
	"net/http"
	"net/http/httptest"
	"testing"

	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
)

func TestAuthorizeCKMK8sProxyUsesCurrentPermission(t *testing.T) {
	p := &configv1alpha1.Permission{}
	request := httptest.NewRequest(http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods/example/log", nil)
	if AuthorizeCKMK8sProxy(p, request) {
		t.Fatal("missing rule granted pod logs")
	}
	p.Spec.RBACRules = []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods/log"}, Verbs: []string{"get"}}}
	if !AuthorizeCKMK8sProxy(p, request) {
		t.Fatal("current permission did not grant pod logs")
	}
	p.Spec.RBACRules = nil
	if AuthorizeCKMK8sProxy(p, request) {
		t.Fatal("removed permission still granted pod logs")
	}
}

func TestAuthorizeCKMK8sProxyDeniesWrongVerbResourceAndPath(t *testing.T) {
	p := &configv1alpha1.Permission{Spec: configv1alpha1.PermissionSpec{RBACRules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}}}}}
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods", true},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods/example", true},
		{http.MethodPost, "/k8s-proxy/api/v1/namespaces/demo/pods", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/secrets/example", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods/example/log", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods?watch=true", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods/%2e%2e/secrets", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods%2fexample", false},
		{http.MethodGet, "/k8s-proxy/api/v1/namespaces/demo/pods/example/exec/extra", false},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := AuthorizeCKMK8sProxy(p, req); got != tc.allowed {
			t.Errorf("%s %s: allowed=%v, want %v", tc.method, tc.path, got, tc.allowed)
		}
	}
}
