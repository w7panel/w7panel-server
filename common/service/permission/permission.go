package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	FounderPermissionName = "k3k.permission.founder"
	AdminPermissionName   = "k3k.permission.admin"
	NormalPermissionName  = "k3k.permission.normal"
)

var permissionGVR = schema.GroupVersionResource{
	Group:    "w7panel.w7.com",
	Version:  "v1alpha1",
	Resource: "permissions",
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
	return &configv1alpha1.Permission{
		Spec: configv1alpha1.PermissionSpec{
			Menu: menu,
			API:  api,
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
	name := annotations[k3ktypes.W7_MENU_NAME]
	if name == "" {
		if annotations[k3ktypes.W7_MENU] != "" || annotations["w7.cc/api"] != "" {
			return FromServiceAccount(sa), nil
		}
		return nil, fmt.Errorf("serviceaccount %s 未关联权限", sa.Name)
	}
	p, err := Get(ctx, sdk, name)
	if err == nil {
		return p, nil
	}
	if errors.IsNotFound(err) && name == FounderPermissionName {
		return founderFallback(), nil
	}
	return nil, err
}

func RBACRoleNameForServiceAccount(ctx context.Context, sdk *k8s.Sdk, sa *corev1.ServiceAccount) (string, error) {
	name := sa.GetAnnotations()[k3ktypes.W7_MENU_NAME]
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
	menu, _ := json.Marshal(p.Spec.Menu)
	apiRules, _ := json.Marshal(p.Spec.API)
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

func SyncPermissionAccount(ctx context.Context, sdk *k8s.Sdk, p *configv1alpha1.Permission) error {
	if !IsBuiltin(p) {
		return nil
	}
	client, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	namespace := sdk.GetNamespace()
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
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, client, clusterRole, func() error {
		clusterRole.Rules = p.Spec.RBACRules
		return nil
	}); err != nil {
		return err
	}
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
	}
	_, err = controllerutil.CreateOrPatch(ctx, client, clusterRoleBinding, func() error {
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
		if !IsBuiltin(p) {
			continue
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
	return MatchAPI(p.Spec.API, method, path), nil
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
	if strings.HasSuffix(pattern, "/*") {
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

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func founderFallback() *configv1alpha1.Permission {
	return &configv1alpha1.Permission{
		Spec: configv1alpha1.PermissionSpec{
			Menu: []string{"*"},
			API:  map[string][]string{"*": []string{"*"}},
			Features: configv1alpha1.PermissionFeatures{
				Debug:      true,
				Webshell:   true,
				Fileeditor: true,
			},
		},
	}
}
