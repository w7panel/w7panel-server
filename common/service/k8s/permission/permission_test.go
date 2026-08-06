package permission

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

func TestIsPanelRole(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: FounderPermissionName, want: true},
		{name: SuperPermissionName, want: true},
		{name: NormalPermissionName, want: true},
		{name: APIPermissionName, want: false},
		{name: "zpk-market", want: false},
		{name: "test", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsPanelRole(tt.name); got != tt.want {
				t.Fatalf("IsPanelRole(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestMatchAPI(t *testing.T) {
	tests := []struct {
		name   string
		rules  map[string][]string
		method string
		path   string
		want   bool
	}{
		{
			name:   "wildcard allows any verb",
			rules:  map[string][]string{"*": {"*"}},
			method: "PATCH",
			path:   "/panel-api/v1/helm/releases/demo",
			want:   true,
		},
		{
			name:   "prefix wildcard allows child path",
			rules:  map[string][]string{"/panel-api/v1/helm/*": {"get"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   true,
		},
		{
			name:   "empty verbs deny explicit path",
			rules:  map[string][]string{"*": {"*"}, "/panel-api/v1/helm/releases": {}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
		{
			name:   "longest match wins",
			rules:  map[string][]string{"/panel-api/v1/helm/*": {"get"}, "/panel-api/v1/helm/releases": {"delete"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
		{
			name:   "get accepts list",
			rules:  map[string][]string{"/panel-api/v1/helm/releases": {"list"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   true,
		},
		{
			name:   "post requires create",
			rules:  map[string][]string{"/panel-api/v1/helm/releases": {"update"}},
			method: "POST",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
		{
			name:   "middle wildcard with trailing wildcard matches proxy path",
			rules:  map[string][]string{"/panel-api/v1/namespaces/*/services/*/proxy/*": {"*"}},
			method: "POST",
			path:   "/panel-api/v1/namespaces/default/services/demo/proxy/api",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchAPI(tt.rules, tt.method, tt.path); got != tt.want {
				t.Fatalf("MatchAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveForServiceAccountRequiresPermissionName(t *testing.T) {
	_, err := ResolveForServiceAccount(context.Background(), nil, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "no-panel-permission"},
	})
	if err == nil {
		t.Fatal("expected error when serviceaccount has no permission annotation")
	}
}

func TestResolveForServiceAccountFallsBackForFounderWithoutPermissionName(t *testing.T) {
	p, err := ResolveForServiceAccount(context.Background(), nil, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "legacy-founder",
			Labels: map[string]string{
				k3ktypes.W7_USER_MODE: k3ktypes.W7_USER_MODE_FOUNDER,
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveForServiceAccount() error = %v", err)
	}
	if !MatchAPI(APIMap(p), "GET", "/panel-api/v1/menu") {
		t.Fatal("expected founder fallback to authorize menu request")
	}
	if !containsString(MenuRules(p), "cluster") || !containsString(MenuRules(p), "usermanage/permission") {
		t.Fatalf("founder fallback menu = %v, want full founder menu", MenuRules(p))
	}
}

func TestResolveForServiceAccountUsesAnnotations(t *testing.T) {
	api, _ := json.Marshal(map[string][]string{"/panel-api/v1/apps/*": {"get"}})
	menu, _ := json.Marshal([]string{"app/*"})
	p, err := ResolveForServiceAccount(context.Background(), nil, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-panel-permission",
			Annotations: map[string]string{
				k3ktypes.W7_MENU: string(menu),
				"w7.cc/api":      string(api),
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveForServiceAccount() error = %v", err)
	}
	if !MatchAPI(APIMap(p), "GET", "/panel-api/v1/apps/demo") {
		t.Fatal("expected annotation api rules to authorize request")
	}
}

func TestSyncPermissionResourcesCreatesClusterRoleForCustomPermission(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &configv1alpha1.Permission{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-dev"},
		Spec: configv1alpha1.PermissionSpec{
			Type: "custom",
			RBACRules: []rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list"},
			}},
		},
	}

	if err := syncPermissionResources(context.Background(), client, "default", p); err != nil {
		t.Fatalf("syncPermissionResources() error = %v", err)
	}
	clusterRole := &rbacv1.ClusterRole{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "custom-dev"}, clusterRole); err != nil {
		t.Fatalf("expected ClusterRole to be created: %v", err)
	}
	if got := clusterRole.Rules; len(got) != 1 || got[0].Resources[0] != "pods" {
		t.Fatalf("unexpected ClusterRole rules: %#v", got)
	}
	sa := &corev1.ServiceAccount{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: "custom-dev", Namespace: "default"}, sa); err == nil {
		t.Fatalf("custom permission should not create permission ServiceAccount")
	}
}

func TestSyncPermissionResourcesCreatesBuiltinAccountAndBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &configv1alpha1.Permission{
		ObjectMeta: metav1.ObjectMeta{Name: NormalPermissionName},
		Spec: configv1alpha1.PermissionSpec{
			Type: "builtin",
			Role: NormalPermissionName,
			RBACRules: []rbacv1.PolicyRule{{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get"},
			}},
		},
	}

	if err := syncPermissionResources(context.Background(), client, "default", p); err != nil {
		t.Fatalf("syncPermissionResources() error = %v", err)
	}
	sa := &corev1.ServiceAccount{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: NormalPermissionName, Namespace: "default"}, sa); err != nil {
		t.Fatalf("expected builtin permission ServiceAccount: %v", err)
	}
	if sa.Labels["w7.cc/permission-account"] != "true" {
		t.Fatalf("permission account label = %q", sa.Labels["w7.cc/permission-account"])
	}
	secret := &corev1.Secret{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: NormalPermissionName, Namespace: "default"}, secret); !apierrors.IsNotFound(err) {
		t.Fatalf("normal permission must not create a token secret, got err = %v", err)
	}
	binding := &rbacv1.ClusterRoleBinding{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: NormalPermissionName}, binding); err != nil {
		t.Fatalf("expected builtin permission ClusterRoleBinding: %v", err)
	}
	if len(binding.Subjects) != 1 || binding.Subjects[0].Name != NormalPermissionName || binding.RoleRef.Name != NormalPermissionName {
		t.Fatalf("unexpected ClusterRoleBinding: %#v", binding)
	}
}

func TestSyncPermissionResourcesCreatesAPITokenSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	p := &configv1alpha1.Permission{
		ObjectMeta: metav1.ObjectMeta{Name: APIPermissionName},
		Spec: configv1alpha1.PermissionSpec{
			Type: "builtin",
			Role: APIPermissionName,
		},
	}

	if err := syncPermissionResources(context.Background(), client, "default", p); err != nil {
		t.Fatalf("syncPermissionResources() error = %v", err)
	}
	secret := &corev1.Secret{}
	if err := client.Get(context.Background(), types.NamespacedName{Name: APIPermissionName, Namespace: "default"}, secret); err != nil {
		t.Fatalf("expected api token Secret: %v", err)
	}
	if secret.Type != corev1.SecretTypeServiceAccountToken {
		t.Fatalf("secret type = %q, want %q", secret.Type, corev1.SecretTypeServiceAccountToken)
	}
	if secret.Annotations[corev1.ServiceAccountNameKey] != APIPermissionName {
		t.Fatalf("service account annotation = %q, want %q", secret.Annotations[corev1.ServiceAccountNameKey], APIPermissionName)
	}
}

func TestNormalizePermissionName(t *testing.T) {
	tests := map[string]string{
		"k3k.permission.founder": "founder",
		"permission.super":       "super",
		"k3k.permission.demo":    "demo",
		"demo":                   "demo",
	}
	for in, want := range tests {
		if got := NormalizePermissionName(in); got != want {
			t.Fatalf("NormalizePermissionName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApplyToServiceAccountKeepsAnnotationFormat(t *testing.T) {
	sa := &corev1.ServiceAccount{}
	ApplyToServiceAccount(sa, &configv1alpha1.Permission{
		Spec: configv1alpha1.PermissionSpec{
			MenuRules: []string{"app/apps"},
			APIRules: []configv1alpha1.PermissionAPIRule{{
				Path:   "/panel-api/v1/apps/*",
				Method: []string{"get"},
			}},
		},
	})
	if got := sa.Annotations[k3ktypes.W7_MENU]; got != `["app/apps"]` {
		t.Fatalf("menu annotation = %s, want list json", got)
	}
	if got := sa.Annotations["w7.cc/api"]; got != `{"/panel-api/v1/apps/*":["get"]}` {
		t.Fatalf("api annotation = %s, want map json", got)
	}
}

func TestApplyToServiceAccountBackfillsFounderDefaults(t *testing.T) {
	sa := &corev1.ServiceAccount{}
	ApplyToServiceAccount(sa, &configv1alpha1.Permission{
		ObjectMeta: metav1.ObjectMeta{Name: FounderPermissionName},
		Spec: configv1alpha1.PermissionSpec{
			Role: FounderPermissionName,
		},
	})
	if got := sa.Annotations["w7.cc/api"]; got != `{"*":["*"]}` {
		t.Fatalf("founder api annotation = %s, want wildcard api", got)
	}
	if got := sa.Annotations[k3ktypes.K3K_DEBUG]; got != "true" {
		t.Fatalf("founder debug annotation = %s, want true", got)
	}
}

func TestFounderFallbackIncludesGatewayPluginPermissions(t *testing.T) {
	menu := MenuRules(founderFallback())
	for _, want := range []string{
		"gateway/plugins",
		"gateway/plugins/add",
		"gateway/plugins/edit",
		"gateway/plugins/delete",
	} {
		found := false
		for _, item := range menu {
			if item == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("founder menu does not include %q", want)
		}
	}
}

func TestIsBuiltin(t *testing.T) {
	tests := []struct {
		name string
		p    *configv1alpha1.Permission
		want bool
	}{
		{
			name: "spec type builtin",
			p:    &configv1alpha1.Permission{Spec: configv1alpha1.PermissionSpec{Type: "builtin"}},
			want: true,
		},
		{
			name: "legacy typemode in",
			p: &configv1alpha1.Permission{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"typemode": "in"}},
			},
			want: true,
		},
		{
			name: "custom",
			p: &configv1alpha1.Permission{
				Spec:       configv1alpha1.PermissionSpec{Type: "custom"},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"typemode": "custom"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltin(tt.p); got != tt.want {
				t.Fatalf("IsBuiltin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuiltinAdminPermissionsDoNotGrantFounderWildcards(t *testing.T) {
	for _, name := range []string{"super.yaml", "api.yaml"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "kodata", "yaml", "permission", name))
			if err != nil {
				if os.IsNotExist(err) {
					t.Skipf("builtin permission file %s does not exist", name)
				}
				t.Fatalf("read builtin permission: %v", err)
			}
			content := string(data)
			if strings.Contains(content, "  api:\n") || strings.Contains(content, "  menu:\n") || strings.Contains(content, "  menu: []") {
				t.Fatal("builtin permissions must use menuRules/apiRules")
			}
			if strings.Contains(content, "  - path: '*'\n    method:\n    - '*'") {
				t.Fatal("admin permission must not grant every panel API")
			}
			if strings.Contains(content, "  - apiGroups:\n    - '*'\n    resources:\n    - '*'\n    verbs:\n    - '*'") {
				t.Fatal("admin permission must not grant founder-level Kubernetes RBAC")
			}
		})
	}
}

func TestBuiltinFounderUsesGatewayTrafficMenu(t *testing.T) {
	p := loadBuiltinPermission(t, "founder.yaml")
	menuRules := MenuRules(p)
	if !containsString(menuRules, "gateway/traffic") {
		t.Fatal("founder permission should include gateway/traffic")
	}
	if containsString(menuRules, "cluster/traffic") {
		t.Fatal("founder permission should not include legacy cluster/traffic")
	}
}

func TestBuiltinNormalPermissionHasNoPanelOrKubernetesPermission(t *testing.T) {
	p := loadBuiltinPermission(t, "normal.yaml")
	api := APIMap(p)

	if len(MenuRules(p)) != 0 {
		t.Fatalf("normal menuRules = %v, want empty", MenuRules(p))
	}
	if len(p.Spec.RBACRules) != 0 {
		t.Fatalf("normal rbacRules = %v, want empty", p.Spec.RBACRules)
	}
	if len(api) != 0 {
		t.Fatalf("normal apiRules = %v, want empty", api)
	}

	denied := []struct {
		method string
		path   string
	}{
		{method: "GET", path: "/panel-api/v1/menu"},
		{method: "GET", path: "/panel-api/v1/auth/permissions/routes"},
		{method: "POST", path: "/panel-api/v1/auth/reset-password-current"},
		{method: "GET", path: "/panel-api/v1/app-info"},
		{method: "GET", path: "/panel-api/v1/kubeconfig"},
		{method: "GET", path: "/panel-api/v1/namespaces"},
		{method: "GET", path: "/panel-api/v1/helm/releases"},
		{method: "GET", path: "/panel-api/v1/audit/list"},
	}
	for _, tt := range denied {
		t.Run("deny "+tt.method+" "+tt.path, func(t *testing.T) {
			if MatchAPI(api, tt.method, tt.path) {
				t.Fatalf("normal permission should deny %s %s", tt.method, tt.path)
			}
		})
	}
}

func TestBuiltinAPIRBACAllowsRegularResourcesOnly(t *testing.T) {
	p := loadBuiltinPermission(t, "api.yaml")

	for _, item := range []struct {
		group    string
		resource string
		verb     string
	}{
		{group: "", resource: "pods", verb: "create"},
		{group: "", resource: "configmaps", verb: "update"},
		{group: "", resource: "secrets", verb: "delete"},
		{group: "", resource: "services", verb: "patch"},
		{group: "apps", resource: "deployments", verb: "create"},
		{group: "batch", resource: "jobs", verb: "delete"},
		{group: "networking.k8s.io", resource: "ingresses", verb: "update"},
	} {
		if !rbacAllows(p.Spec.RBACRules, item.group, item.resource, item.verb) {
			t.Fatalf("api rbac should allow %s for %s/%s", item.verb, item.group, item.resource)
		}
	}

	for _, item := range []struct {
		group    string
		resource string
	}{
		{group: "apiextensions.k8s.io", resource: "customresourcedefinitions"},
		{group: "", resource: "namespaces"},
		{group: "", resource: "serviceaccounts"},
		{group: "w7panel.w7.com", resource: "permissions"},
	} {
		if !rbacAllows(p.Spec.RBACRules, item.group, item.resource, "get") {
			t.Fatalf("api rbac should allow read for %s/%s", item.group, item.resource)
		}
		for _, verb := range []string{"create", "update", "patch", "delete"} {
			if rbacAllows(p.Spec.RBACRules, item.group, item.resource, verb) {
				t.Fatalf("api rbac must not allow %s for %s/%s", verb, item.group, item.resource)
			}
		}
	}
}

func TestBuiltinSuperPermissionExcludesTenantAndSystemManagement(t *testing.T) {
	p := loadBuiltinPermission(t, "super.yaml")
	api := APIMap(p)

	for _, denied := range []string{
		"usermanage/*",
		"usermanage/site-setting",
		"system/license",
		"system/audit",
	} {
		if containsString(MenuRules(p), denied) {
			t.Fatalf("super menuRules must not include %q", denied)
		}
	}
	if !containsString(MenuRules(p), "system/cloud") {
		t.Fatal("super menuRules must keep system/cloud")
	}

	deniedAPI := []struct {
		method string
		path   string
	}{
		{method: "POST", path: "/panel-api/v1/auth/reset-password"},
		{method: "GET", path: "/panel-api/v1/audit/login-logs"},
		{method: "POST", path: "/panel-api/v1/oidc/register"},
	}
	for _, tt := range deniedAPI {
		if MatchAPI(api, tt.method, tt.path) {
			t.Fatalf("super permission should deny %s %s", tt.method, tt.path)
		}
	}
	if !MatchAPI(api, "POST", "/panel-api/v1/auth/console/register-to-console") {
		t.Fatal("super permission should keep cloud registration APIs")
	}
}

func TestBuiltinSuperRBACRestrictsSensitiveResourcesToReadOnly(t *testing.T) {
	p := loadBuiltinPermission(t, "super.yaml")

	sensitive := []struct {
		group    string
		resource string
	}{
		{group: "apiextensions.k8s.io", resource: "customresourcedefinitions"},
		{group: "", resource: "namespaces"},
		{group: "", resource: "serviceaccounts"},
		{group: "w7panel.w7.com", resource: "users"},
		{group: "w7panel.w7.com", resource: "permissions"},
	}
	for _, item := range sensitive {
		if !rbacAllows(p.Spec.RBACRules, item.group, item.resource, "get") {
			t.Fatalf("super rbac should allow read for %s/%s", item.group, item.resource)
		}
		for _, verb := range []string{"create", "update", "patch", "delete"} {
			if rbacAllows(p.Spec.RBACRules, item.group, item.resource, verb) {
				t.Fatalf("super rbac must not allow %s for %s/%s", verb, item.group, item.resource)
			}
		}
	}

	if !rbacAllows(p.Spec.RBACRules, "apps", "deployments", "create") {
		t.Fatal("super rbac should allow non-sensitive Kubernetes resources")
	}
	if !rbacAllows(p.Spec.RBACRules, "", "pods", "delete") {
		t.Fatal("super rbac should allow regular core resources")
	}
}

func loadBuiltinPermission(t *testing.T, name string) *configv1alpha1.Permission {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "kodata", "yaml", "permission", name))
	if err != nil {
		t.Fatalf("read builtin permission %s: %v", name, err)
	}
	p := &configv1alpha1.Permission{}
	if err := yaml.Unmarshal(data, p); err != nil {
		t.Fatalf("parse builtin permission %s: %v", name, err)
	}
	return p
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func rbacAllows(rules []rbacv1.PolicyRule, apiGroup, resource, verb string) bool {
	for _, rule := range rules {
		if containsString(rule.APIGroups, apiGroup) || containsString(rule.APIGroups, "*") {
			if containsString(rule.Resources, resource) || containsString(rule.Resources, "*") {
				if containsString(rule.Verbs, verb) || containsString(rule.Verbs, "*") {
					return true
				}
			}
		}
	}
	return false
}
