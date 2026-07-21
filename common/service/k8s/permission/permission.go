package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FounderPermissionName = "founder"
	SuperPermissionName   = "super"
	NormalPermissionName  = "normal"
	APIPermissionName     = "api"
)

var permissionGVR = schema.GroupVersionResource{
	Group:    "w7panel.w7.com",
	Version:  "v1alpha1",
	Resource: "permissions",
}

var founderMenu = []string{
	"cluster",
	"cluster/panel",
	"cluster/resource",
	"cluster/dns",
	"app",
	"app/apps",
	"app/apps/add",
	"app/apps/edit",
	"app/apps/delete",
	"app/cronjob",
	"app/cronjob/add",
	"app/cronjob/edit",
	"app/cronjob/delete",
	"app/rvproxy",
	"app/rvproxy/add",
	"app/rvproxy/edit",
	"app/rvproxy/delete",
	"gateway",
	"gateway/rvproxy",
	"gateway/rvproxy/add",
	"gateway/rvproxy/edit",
	"gateway/rvproxy/delete",
	"gateway/aiproxy",
	"gateway/aiproxy/add",
	"gateway/aiproxy/edit",
	"gateway/aiproxy/delete",
	"gateway/plugins",
	"gateway/plugins/add",
	"gateway/plugins/edit",
	"gateway/plugins/delete",
	"app/database",
	"app/database/add",
	"app/database/delete",
	"app/gpustack",
	"storage",
	"storage/disk",
	"storage/disk/add",
	"storage/disk/edit",
	"storage/disk/delete",
	"storage/zone",
	"zpk",
	"sitemanage",
	"system",
	"system/cloud",
	"system/license",
	"system/audit",
	"person/order-center",
	"person/cost-center",
	"cluster/nodes",
	"cluster/nodes/add",
	"cluster/nodes/registries",
	"cluster/nodes-image-list",
	"cluster/nodes/gpu",
	"cluster/nodes/memory",
	"usermanage/usermanage-whitedomain",
	"usermanage",
	"usermanage/users",
	"usermanage/usergroup",
	"usermanage/permission",
	"usermanage/quota",
	"usermanage/usermanage-system",
}

func Get(ctx context.Context, sdk *k8s.Sdk, name string) (*configv1alpha1.Permission, error) {
	obj, err := sdk.DynamicClient().Resource(permissionGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	permission := &configv1alpha1.Permission{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, permission); err != nil {
		return nil, err
	}
	return permission, nil
}

func FromServiceAccount(sa *corev1.ServiceAccount) *configv1alpha1.Permission {
	annotations := sa.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	menu := []string{}
	_ = json.Unmarshal([]byte(annotations[k3ktypes.W7_MENU]), &menu)
	api := map[string][]string{}
	_ = json.Unmarshal([]byte(annotations["w7.cc/api"]), &api)
	whiteList := []configv1alpha1.DomainWhiteItem{}
	_ = json.Unmarshal([]byte(annotations[k3ktypes.W7_DOMAIN_WHITE_LIST]), &whiteList)
	return &configv1alpha1.Permission{
		Spec: configv1alpha1.PermissionSpec{
			MenuRules:       menu,
			APIRules:        APIMapToRules(api),
			DomainWhiteList: whiteList,
			Features: configv1alpha1.PermissionFeatures{
				Debug:      annotations[k3ktypes.K3K_DEBUG] == "true",
				Webshell:   annotations[k3ktypes.W7_WEB_SHELL] == "true",
				Fileeditor: annotations[k3ktypes.W7_FILE_EDITTOR] == "true",
			},
		},
	}
}

func ResolveForServiceAccount(ctx context.Context, sdk *k8s.Sdk, sa *corev1.ServiceAccount) (*configv1alpha1.Permission, error) {
	annotations := sa.GetAnnotations()
	name := NormalizePermissionName(annotations[k3ktypes.W7_MENU_NAME])
	if name == "" {
		if isFounderServiceAccount(sa) {
			return founderFallback(), nil
		}
		if isNormalServiceAccount(sa) && sdk != nil {
			if p, err := Get(ctx, sdk, NormalPermissionName); err == nil {
				return p, nil
			}
		}
		if annotations[k3ktypes.W7_MENU] != "" || annotations["w7.cc/api"] != "" {
			return FromServiceAccount(sa), nil
		}
		return nil, fmt.Errorf("serviceaccount %s 未关联权限", sa.Name)
	}
	p, err := Get(ctx, sdk, name)
	if err == nil {
		EnsureBuiltinDefaults(p)
		return p, nil
	}
	if errors.IsNotFound(err) && name == FounderPermissionName {
		return founderFallback(), nil
	}
	return nil, err
}

func RBACRoleNameForServiceAccount(ctx context.Context, sdk *k8s.Sdk, sa *corev1.ServiceAccount) (string, error) {
	name := NormalizePermissionName(sa.GetAnnotations()[k3ktypes.W7_MENU_NAME])
	if name == "" {
		return "", nil
	}
	p, err := ResolveForServiceAccount(ctx, sdk, sa)
	if err != nil {
		return "", err
	}
	if !IsBuiltin(p) && p.Spec.ParentPermission != "" {
		return p.Spec.ParentPermission, nil
	}
	if p.Name != "" {
		return p.Name, nil
	}
	return name, nil
}

func ApplyToServiceAccount(sa *corev1.ServiceAccount, p *configv1alpha1.Permission) {
	if sa.Annotations == nil {
		sa.Annotations = map[string]string{}
	}
	if sa.Labels == nil {
		sa.Labels = map[string]string{}
	}
	EnsureBuiltinDefaults(p)
	menu, _ := json.Marshal(MenuRules(p))
	apiRules, _ := json.Marshal(APIMap(p))
	whiteList, _ := json.Marshal(p.Spec.DomainWhiteList)
	sa.Annotations[k3ktypes.W7_MENU] = string(menu)
	sa.Annotations["w7.cc/api"] = string(apiRules)
	sa.Annotations[k3ktypes.K3K_DEBUG] = boolString(p.Spec.Features.Debug)
	sa.Annotations[k3ktypes.W7_WEB_SHELL] = boolString(p.Spec.Features.Webshell)
	sa.Annotations[k3ktypes.W7_FILE_EDITTOR] = boolString(p.Spec.Features.Fileeditor)
	sa.Annotations[k3ktypes.W7_DOMAIN_WHITE_LIST] = string(whiteList)
	if p.Spec.Role != "" {
		sa.Labels[k3ktypes.W7_ROLE] = p.Spec.Role
	}
}

func NormalizePermissionName(name string) string {
	switch name {
	case "k3k.permission.founder", "permission.founder":
		return FounderPermissionName
	case "k3k.permission.super", "permission.super":
		return SuperPermissionName
	case "k3k.permission.normal", "permission.normal":
		return NormalPermissionName
	case "k3k.permission.api", "permission.api":
		return APIPermissionName
	default:
		return strings.TrimPrefix(strings.TrimPrefix(name, "k3k.permission."), "permission.")
	}
}

func SyncPermissionAccount(ctx context.Context, sdk *k8s.Sdk, p *configv1alpha1.Permission) error {
	client, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	return syncPermissionResources(ctx, client, sdk.GetNamespace(), p)
}

func syncPermissionResources(ctx context.Context, client client.Client, namespace string, p *configv1alpha1.Permission) error {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, client, clusterRole, func() error {
		clusterRole.Rules = p.Spec.RBACRules
		return nil
	}); err != nil {
		return err
	}
	if !IsBuiltin(p) {
		return nil
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name,
			Namespace: namespace,
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, client, sa, func() error {
		if sa.Labels == nil {
			sa.Labels = map[string]string{}
		}
		sa.Labels["w7.cc/permission-account"] = "true"
		sa.Labels[k3ktypes.W7_ROLE] = p.Spec.Role
		if sa.Annotations == nil {
			sa.Annotations = map[string]string{}
		}
		sa.Annotations[k3ktypes.W7_MENU_NAME] = p.Name
		ApplyToServiceAccount(sa, p)
		return nil
	}); err != nil {
		return err
	}
	if p.Name == APIPermissionName {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.Name,
				Namespace: namespace,
			},
		}
		if _, err := controllerutil.CreateOrPatch(ctx, client, secret, func() error {
			if secret.Type != "" && secret.Type != corev1.SecretTypeServiceAccountToken {
				return fmt.Errorf("api token secret %s has unexpected type %q", secret.Name, secret.Type)
			}
			if secret.Annotations == nil {
				secret.Annotations = map[string]string{}
			}
			if serviceAccountName := secret.Annotations[corev1.ServiceAccountNameKey]; serviceAccountName != "" && serviceAccountName != p.Name {
				return fmt.Errorf("api token secret %s belongs to service account %q", secret.Name, serviceAccountName)
			}
			secret.Type = corev1.SecretTypeServiceAccountToken
			secret.Annotations[corev1.ServiceAccountNameKey] = p.Name
			return nil
		}); err != nil {
			return err
		}
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
	}
	_, err := controllerutil.CreateOrPatch(ctx, client, clusterRoleBinding, func() error {
		clusterRoleBinding.Subjects = []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      p.Name,
			Namespace: namespace,
		}}
		clusterRoleBinding.RoleRef = rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     p.Name,
			APIGroup: "rbac.authorization.k8s.io",
		}
		return nil
	})
	return err
}

func SyncAllPermissionAccounts(ctx context.Context, sdk *k8s.Sdk) error {
	list, err := sdk.DynamicClient().Resource(permissionGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		p := &configv1alpha1.Permission{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.Object, p); err != nil {
			return err
		}
		if err := SyncPermissionAccount(ctx, sdk, p); err != nil {
			return err
		}
	}
	return nil
}

func AuthorizePanelAPI(ctx context.Context, sdk *k8s.Sdk, saName, method, path string) (bool, error) {
	if !strings.HasPrefix(path, "/panel-api/v1/") {
		return true, nil
	}
	if isAlwaysAllowed(path) {
		return true, nil
	}
	sa, err := sdk.GetServiceAccount(sdk.GetNamespace(), saName)
	if err != nil {
		return false, err
	}
	p, err := ResolveForServiceAccount(ctx, sdk, sa)
	if err != nil {
		return false, err
	}
	return MatchAPI(APIMap(p), method, path), nil
}

func AuthorizePanelAPIWithPermission(ctx context.Context, sdk *k8s.Sdk, p *configv1alpha1.Permission, method, path string) (bool, error) {
	if !strings.HasPrefix(path, "/panel-api/v1/") {
		return true, nil
	}
	if isAlwaysAllowed(path) {
		return true, nil
	}
	if p == nil {
		return false, fmt.Errorf("用户未关联权限")
	}
	EnsureBuiltinDefaults(p)
	return MatchAPI(APIMap(p), method, path), nil
}

func MenuRules(p *configv1alpha1.Permission) []string {
	if p == nil {
		return nil
	}
	return p.Spec.MenuRules
}

func APIRules(p *configv1alpha1.Permission) []configv1alpha1.PermissionAPIRule {
	if p == nil {
		return nil
	}
	return p.Spec.APIRules
}

func APIMap(p *configv1alpha1.Permission) map[string][]string {
	return APIRulesToMap(APIRules(p))
}

func APIMapToRules(api map[string][]string) []configv1alpha1.PermissionAPIRule {
	if api == nil {
		return nil
	}
	rules := make([]configv1alpha1.PermissionAPIRule, 0, len(api))
	for path, methods := range api {
		rule := configv1alpha1.PermissionAPIRule{
			Path:   path,
			Method: append([]string(nil), methods...),
		}
		rules = append(rules, rule)
	}
	return rules
}

func APIRulesToMap(rules []configv1alpha1.PermissionAPIRule) map[string][]string {
	api := make(map[string][]string, len(rules))
	for _, rule := range rules {
		if rule.Path == "" {
			continue
		}
		api[rule.Path] = append([]string(nil), rule.Method...)
	}
	return api
}

func EnsureBuiltinDefaults(p *configv1alpha1.Permission) {
	if p == nil || NormalizePermissionName(p.Name) != FounderPermissionName {
		return
	}
	if len(p.Spec.APIRules) == 0 {
		p.Spec.APIRules = []configv1alpha1.PermissionAPIRule{{
			Path:   "*",
			Method: []string{"*"},
		}}
	}
	if len(p.Spec.RBACRules) == 0 {
		p.Spec.RBACRules = []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		}}
	}
	if !p.Spec.Features.Debug && !p.Spec.Features.Webshell && !p.Spec.Features.Fileeditor {
		p.Spec.Features = configv1alpha1.PermissionFeatures{
			Debug:      true,
			Webshell:   true,
			Fileeditor: true,
		}
	}
}

func MatchAPI(rules map[string][]string, method, path string) bool {
	if len(rules) == 0 {
		return false
	}
	bestPattern := ""
	var bestVerbs []string
	matched := false
	for pattern, verbs := range rules {
		if !matchPath(pattern, path) {
			continue
		}
		if !matched || len(pattern) > len(bestPattern) {
			bestPattern = pattern
			bestVerbs = verbs
			matched = true
		}
	}
	if !matched || len(bestVerbs) == 0 {
		return false
	}
	for _, required := range verbsForMethod(method) {
		if containsVerb(bestVerbs, required) {
			return true
		}
	}
	return false
}

func matchPath(pattern, path string) bool {
	if pattern == "*" || pattern == path {
		return true
	}
	if strings.HasSuffix(pattern, "/*") && !strings.Contains(strings.TrimSuffix(pattern, "/*"), "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "/*")+"/")
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return false
	}
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(path[pos:], part)
		if idx < 0 {
			return false
		}
		if i == 0 && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(path, last)
}

func verbsForMethod(method string) []string {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return []string{"get", "list"}
	case "POST":
		return []string{"create"}
	case "PUT":
		return []string{"update"}
	case "PATCH":
		return []string{"patch"}
	case "DELETE":
		return []string{"delete"}
	default:
		return []string{strings.ToLower(method)}
	}
}

func containsVerb(verbs []string, required string) bool {
	for _, verb := range verbs {
		verb = strings.ToLower(verb)
		if verb == "*" || verb == required {
			return true
		}
	}
	return false
}

func isAlwaysAllowed(path string) bool {
	return strings.HasPrefix(path, "/panel-api/v1/noauth/") ||
		path == "/panel-api/v1/auth/userinfo" ||
		path == "/panel-api/v1/k3k/info" ||
		path == "/panel-api/v1/auth/refresh-token2"
}

func IsBuiltin(p *configv1alpha1.Permission) bool {
	if p == nil {
		return false
	}
	if p.Spec.Type == "builtin" {
		return true
	}
	return p.Labels["typemode"] == "in"
}

func isFounderServiceAccount(sa *corev1.ServiceAccount) bool {
	labels := sa.GetLabels()
	if labels[k3ktypes.W7_USER_MODE] == k3ktypes.W7_USER_MODE_FOUNDER {
		return true
	}
	if labels[k3ktypes.W7_ROLE] == k3ktypes.W7_USER_MODE_FOUNDER {
		return true
	}
	return false
}

func isNormalServiceAccount(sa *corev1.ServiceAccount) bool {
	labels := sa.GetLabels()
	return labels[k3ktypes.W7_USER_MODE] == k3ktypes.W7_USER_MODE_NORMAL ||
		labels[k3ktypes.K3K_USER_MODE] == k3ktypes.W7_USER_MODE_NORMAL ||
		labels[k3ktypes.W7_ROLE] == k3ktypes.W7_USER_MODE_NORMAL
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func founderFallback() *configv1alpha1.Permission {
	return &configv1alpha1.Permission{
		Spec: configv1alpha1.PermissionSpec{
			MenuRules: founderMenu,
			APIRules: []configv1alpha1.PermissionAPIRule{{
				Path:   "*",
				Method: []string{"*"},
			}},
			Features: configv1alpha1.PermissionFeatures{
				Debug:      true,
				Webshell:   true,
				Fileeditor: true,
			},
		},
	}
}
