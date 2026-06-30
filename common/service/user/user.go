package user

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	userv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Kind       = userv1alpha1.Kind
	APIVersion = userv1alpha1.APIVersion
)

var GVR = userv1alpha1.GVR

type Spec = userv1alpha1.UserSpec
type W7Config = userv1alpha1.W7Config

type User struct {
	Name      string
	Spec      Spec
	Object    *unstructured.Unstructured
	CreatedAt time.Time
}

func Get(ctx context.Context, sdk *k8s.Sdk, name string) (*User, error) {
	obj, err := sdk.DynamicClient().Resource(GVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return FromUnstructured(obj)
}

func GetByConsoleID(ctx context.Context, sdk *k8s.Sdk, consoleID string) (*User, error) {
	list, err := sdk.DynamicClient().Resource(GVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		u, err := FromUnstructured(&list.Items[i])
		if err == nil && u.Spec.ConsoleId == consoleID {
			return u, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: GVR.Group, Resource: GVR.Resource}, consoleID)
}

func FromUnstructured(obj *unstructured.Unstructured) (*User, error) {
	spec := Spec{}
	if raw, ok := obj.Object["spec"].(map[string]interface{}); ok {
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
			return nil, err
		}
	}
	return &User{Name: obj.GetName(), Spec: spec, Object: obj, CreatedAt: obj.GetCreationTimestamp().Time}, nil
}

func ToUnstructured(name string, spec Spec) (*unstructured.Unstructured, error) {
	specMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&spec)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": APIVersion,
		"kind":       Kind,
		"metadata": map[string]interface{}{
			"name": name,
		},
		"spec": specMap,
	}}, nil
}

func Create(ctx context.Context, sdk *k8s.Sdk, name, password string, spec Spec) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	spec.PasswordHash = string(hash)
	spec = normalizeSpec(name, spec)
	obj, err := ToUnstructured(name, spec)
	if err != nil {
		return nil, err
	}
	created, err := sdk.DynamicClient().Resource(GVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return FromUnstructured(created)
}

func ResetPassword(ctx context.Context, sdk *k8s.Sdk, name, password string) error {
	u, err := Get(ctx, sdk, name)
	if err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.Spec.PasswordHash = string(hash)
	obj, err := ToUnstructured(name, u.Spec)
	if err != nil {
		return err
	}
	obj.SetResourceVersion(u.Object.GetResourceVersion())
	_, err = sdk.DynamicClient().Resource(GVR).Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func Login(ctx context.Context, sdk *k8s.Sdk, username, password string) (*User, error) {
	u, err := Get(ctx, sdk, username)
	if err != nil {
		return nil, err
	}
	if err := checkExpire(u); err != nil {
		return nil, err
	}
	if u.Spec.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(u.Spec.PasswordHash), []byte(password)) != nil {
		return nil, fmt.Errorf("用户名密码错误")
	}
	return u, nil
}

func ResolvePermission(ctx context.Context, sdk *k8s.Sdk, u *User) (*configv1alpha1.Permission, error) {
	name := permissionservice.NormalizePermissionName(u.Spec.PermissionName)
	if name != "" {
		p, err := permissionservice.Get(ctx, sdk, name)
		if err == nil {
			permissionservice.EnsureBuiltinDefaults(p)
			return p, nil
		}
		return nil, err
	}
	if len(u.Spec.MenuRules) > 0 || len(u.Spec.APIRules) > 0 {
		return &configv1alpha1.Permission{Spec: configv1alpha1.PermissionSpec{
			Role:            role(u),
			MenuRules:       append([]string(nil), u.Spec.MenuRules...),
			APIRules:        append([]configv1alpha1.PermissionAPIRule(nil), u.Spec.APIRules...),
			Features:        u.Spec.Features,
			DomainWhiteList: append([]string(nil), u.Spec.DomainWhiteList...),
		}}, nil
	}
	switch role(u) {
	case permissionservice.FounderPermissionName:
		return permissionservice.Get(ctx, sdk, permissionservice.FounderPermissionName)
	case permissionservice.SuperPermissionName:
		return permissionservice.Get(ctx, sdk, permissionservice.SuperPermissionName)
	default:
		return permissionservice.Get(ctx, sdk, permissionservice.NormalPermissionName)
	}
}

func ExecutionServiceAccount(ctx context.Context, sdk *k8s.Sdk, u *User) (string, error) {
	p, err := ResolvePermission(ctx, sdk, u)
	if err != nil {
		return "", err
	}
	if p.Name != "" {
		if p.Spec.ParentPermission != "" {
			return permissionservice.NormalizePermissionName(p.Spec.ParentPermission), nil
		}
		return p.Name, nil
	}
	if u.Spec.PermissionName != "" {
		return permissionservice.NormalizePermissionName(u.Spec.PermissionName), nil
	}
	r := role(u)
	if r == "" {
		return permissionservice.NormalPermissionName, nil
	}
	return r, nil
}

func ToServiceAccount(u *User, namespace string) *corev1.ServiceAccount {
	annotations := map[string]string{
		"password":                        u.Spec.PasswordHash,
		k3ktypes.W7_MENU_NAME:             u.Spec.PermissionName,
		k3ktypes.K3K_DEBUG:                boolString(u.Spec.Features.Debug),
		k3ktypes.W7_WEB_SHELL:             boolString(u.Spec.Features.Webshell),
		k3ktypes.W7_FILE_EDITTOR:          boolString(u.Spec.Features.Fileeditor),
		k3ktypes.W7_DOMAIN_WHITE_LIST:     mustJSON(u.Spec.DomainWhiteList),
		"w7.cc/api":                       mustJSON(permissionservice.APIRulesToMap(u.Spec.APIRules)),
		k3ktypes.W7_MENU:                  mustJSON(u.Spec.MenuRules),
		"w7.cc/expiretime":                u.Spec.ExpireTime,
		"w7.cc/login-time":                u.Spec.LoginTime,
		"w7.cc/console-openid":            u.Spec.ConsoleOpenid,
		"w7.cc/console-nickname":          u.Spec.ConsoleNickname,
		k3ktypes.K3K_PENDING_RECYCLE_TIME: u.Spec.PendingRecycle,
		k3ktypes.W7_PAUSE:                 u.Spec.Pause,
		k3ktypes.K3K_JOB_NAME:             u.Spec.JobName,
	}
	labels := map[string]string{
		k3ktypes.W7_USER_MODE: role(u),
		k3ktypes.W7_ROLE:      role(u),
		"w7.cc/demo-user":     boolString(u.Spec.DemoUser),
		k3ktypes.W7_WH_MODE:   boolString(u.Spec.Maintenance),
	}
	if u.Spec.ConsoleId != "" {
		labels[k3ktypes.W7_CONSOLE_ID] = u.Spec.ConsoleId
	}
	return &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name:        u.Name,
		Namespace:   namespace,
		Labels:      labels,
		Annotations: annotations,
	}}
}

func FromServiceAccount(sa *corev1.ServiceAccount) Spec {
	annotations := sa.GetAnnotations()
	labels := sa.GetLabels()
	menu := []string{}
	_ = json.Unmarshal([]byte(annotations[k3ktypes.W7_MENU]), &menu)
	api := map[string][]string{}
	_ = json.Unmarshal([]byte(annotations["w7.cc/api"]), &api)
	whiteList := []string{}
	_ = json.Unmarshal([]byte(annotations[k3ktypes.W7_DOMAIN_WHITE_LIST]), &whiteList)
	return normalizeSpec(sa.Name, Spec{
		PasswordHash:   annotations["password"],
		UserMode:       labels[k3ktypes.W7_USER_MODE],
		Role:           labels[k3ktypes.W7_ROLE],
		PermissionName: annotations[k3ktypes.W7_MENU_NAME],
		MenuRules:      menu,
		APIRules:       permissionservice.APIMapToRules(api),
		Features: configv1alpha1.PermissionFeatures{
			Debug:      annotations[k3ktypes.K3K_DEBUG] == "true",
			Webshell:   annotations[k3ktypes.W7_WEB_SHELL] == "true",
			Fileeditor: annotations[k3ktypes.W7_FILE_EDITTOR] == "true",
		},
		DomainWhiteList: whiteList,
		ExpireTime:      annotations["w7.cc/expiretime"],
		DemoUser:        labels["w7.cc/demo-user"] == "true",
		ConsoleId:       labels[k3ktypes.W7_CONSOLE_ID],
		ConsoleOpenid:   annotations["w7.cc/console-openid"],
		ConsoleNickname: annotations["w7.cc/console-nickname"],
		LoginTime:       annotations["w7.cc/login-time"],
		Status:          annotations["w7.cc/k3k-job-status"],
		Pause:           annotations[k3ktypes.W7_PAUSE],
		JobName:         annotations[k3ktypes.K3K_JOB_NAME],
		PendingRecycle:  annotations[k3ktypes.K3K_PENDING_RECYCLE_TIME],
		Maintenance:     labels[k3ktypes.W7_WH_MODE] == "true",
	})
}

func MigrateServiceAccounts(ctx context.Context, sdk *k8s.Sdk) error {
	list, err := sdk.ClientSet.CoreV1().ServiceAccounts(sdk.GetNamespace()).List(ctx, metav1.ListOptions{LabelSelector: "w7.cc/user-mode"})
	if err != nil {
		return err
	}
	for i := range list.Items {
		sa := &list.Items[i]
		if _, err := Get(ctx, sdk, sa.Name); err == nil {
			continue
		}
		if _, err := CreateWithHash(ctx, sdk, sa.Name, FromServiceAccount(sa)); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}

func CreateWithHash(ctx context.Context, sdk *k8s.Sdk, name string, spec Spec) (*User, error) {
	spec = normalizeSpec(name, spec)
	obj, err := ToUnstructured(name, spec)
	if err != nil {
		return nil, err
	}
	created, err := sdk.DynamicClient().Resource(GVR).Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return FromUnstructured(created)
}

func normalizeSpec(name string, spec Spec) Spec {
	if spec.UserMode == "" {
		spec.UserMode = spec.Role
	}
	if spec.UserMode == "" {
		spec.UserMode = permissionservice.NormalPermissionName
	}
	if spec.Role == "" {
		spec.Role = spec.UserMode
	}
	if spec.PermissionName == "" {
		switch spec.UserMode {
		case permissionservice.FounderPermissionName, permissionservice.SuperPermissionName, permissionservice.NormalPermissionName:
			spec.PermissionName = spec.UserMode
		}
	}
	if name == "admin" && spec.UserMode == "" {
		spec.UserMode = permissionservice.FounderPermissionName
		spec.Role = permissionservice.FounderPermissionName
		spec.PermissionName = permissionservice.FounderPermissionName
	}
	return spec
}

func checkExpire(u *User) error {
	if strings.TrimSpace(u.Spec.ExpireTime) == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", u.Spec.ExpireTime, time.Local)
	if err != nil {
		return err
	}
	if t.Unix() < time.Now().Unix() && role(u) != "cluster" && role(u) != "founder" {
		return fmt.Errorf("用户已过期")
	}
	return nil
}

func role(u *User) string {
	if u == nil {
		return permissionservice.NormalPermissionName
	}
	if u.Spec.Role != "" {
		return u.Spec.Role
	}
	if u.Spec.UserMode != "" {
		return u.Spec.UserMode
	}
	return permissionservice.NormalPermissionName
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
