package console

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type QxUpgrade struct {
	console2.Abstract
}

func (c QxUpgrade) GetName() string {
	return "qx-upgrade"
}

func (c QxUpgrade) Configure(cmd *cobra.Command) {

}

func (c QxUpgrade) GetDescription() string {
	return "升级角色权限"
}

func (c QxUpgrade) Handle(cmd *cobra.Command, args []string) {

	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("Failed to create sigclient", "error", err)
		return
	}
	configmaps, err := sdk.ClientSet.CoreV1().ConfigMaps("default").List(context.Background(), v1.ListOptions{LabelSelector: "type=permission"})
	if err != nil {
		slog.Error("Failed to list configmaps", "error", err)
		return
	}
	c.handleConfigmaps(configmaps, sigClient)
	c.migratePermissions(sdk.Sdk, configmaps)

	configmaps2, err := sdk.ClientSet.CoreV1().ConfigMaps("default").List(context.Background(), v1.ListOptions{LabelSelector: "type=quota"})
	if err != nil {
		slog.Error("Failed to list configmaps", "error", err)
		return
	}
	c.handleConfigmaps(configmaps2, sigClient)
	if err := permissionservice.SyncAllPermissionAccounts(context.Background(), sdk.Sdk); err != nil {
		slog.Error("Failed to sync permission accounts", "error", err)
	}
	if err := c.syncServiceAccountPermissions(sdk.Sdk); err != nil {
		slog.Error("Failed to sync serviceaccount permissions", "error", err)
	}
}

func (QxUpgrade) handleConfigmaps(configmaps *corev1.ConfigMapList, sigClient sigclient.Client) {
	for _, configmap := range configmaps.Items {
		if configmap.Labels == nil {
			configmap.Labels = map[string]string{}
		}
		if configmap.Annotations == nil {
			configmap.Annotations = map[string]string{}
		}
		if permissionservice.NormalizePermissionName(configmap.Name) == permissionservice.FounderPermissionName {
			// configmap.Labels["w7.cc/role"] = "founder"
			configmap.Labels["typemode"] = "in"
			err := sigClient.Update(context.Background(), &configmap)
			if err != nil {
				slog.Error("Failed to update configmap", "error", err)
			}
			continue
		}
		_, ok := configmap.Labels["w7.cc/role"]
		if !ok && configmap.Labels["typemode"] != "in" {
			configmap.Labels["type"] = "permission"
			configmap.Labels["w7.cc/role"] = "normal"
			configmap.Labels["typemode"] = "custom"
			configmap.Annotations["title"] = "[普通用户]" + configmap.Annotations["title"]
			err := sigClient.Update(context.Background(), &configmap)
			if err != nil {
				slog.Error("Failed to update configmap", "error", err)
			}
		}
	}
}

func (QxUpgrade) migratePermissions(sdk *k8s.Sdk, configmaps *corev1.ConfigMapList) {
	gvr := schema.GroupVersionResource{Group: "w7panel.w7.com", Version: "v1alpha1", Resource: "permissions"}
	for _, cm := range configmaps.Items {
		if cm.Name == "k3k.permission.tech" || cm.Name == "permission.tech" || cm.Name == "tech" {
			continue
		}
		p := configMapToPermission(&cm)
		permissionservice.EnsureBuiltinDefaults(p)
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(p)
		if err != nil {
			slog.Error("Failed to convert permission", "error", err, "name", cm.Name)
			continue
		}
		_, err = sdk.DynamicClient().Resource(gvr).Create(context.Background(), &unstructured.Unstructured{Object: obj}, v1.CreateOptions{})
		if errors.IsAlreadyExists(err) {
			current, getErr := sdk.DynamicClient().Resource(gvr).Get(context.Background(), p.Name, v1.GetOptions{})
			if getErr != nil {
				slog.Error("Failed to get existing permission", "error", getErr, "name", p.Name)
				continue
			}
			obj["metadata"].(map[string]interface{})["resourceVersion"] = current.GetResourceVersion()
			_, err = sdk.DynamicClient().Resource(gvr).Update(context.Background(), &unstructured.Unstructured{Object: obj}, v1.UpdateOptions{})
		}
		if err != nil {
			slog.Error("Failed to create permission", "error", err, "name", cm.Name)
		}
	}
}

func (QxUpgrade) syncServiceAccountPermissions(sdk *k8s.Sdk) error {
	list, err := sdk.ClientSet.CoreV1().ServiceAccounts(sdk.GetNamespace()).List(context.Background(), v1.ListOptions{
		LabelSelector: "w7.cc/user-mode",
	})
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		sa := item.DeepCopy()
		permissionName := permissionservice.NormalizePermissionName(sa.GetAnnotations()[k3ktypes.W7_MENU_NAME])
		if permissionName == "" && isNormalServiceAccount(sa) {
			permissionName = permissionservice.NormalPermissionName
		}
		if permissionName == "" {
			continue
		}
		p, err := permissionservice.Get(context.Background(), sdk, permissionName)
		if err != nil {
			slog.Error("Failed to get serviceaccount permission", "error", err, "sa", sa.Name, "permission", permissionName)
			continue
		}
		permissionservice.EnsureBuiltinDefaults(p)
		sa.Annotations[k3ktypes.W7_MENU_NAME] = permissionName
		permissionservice.ApplyToServiceAccount(sa, p)
		if _, err := sdk.ClientSet.CoreV1().ServiceAccounts(sa.Namespace).Update(context.Background(), sa, v1.UpdateOptions{}); err != nil {
			slog.Error("Failed to update serviceaccount permission", "error", err, "sa", sa.Name)
		}
	}
	return nil
}

func isNormalServiceAccount(sa *corev1.ServiceAccount) bool {
	labels := sa.GetLabels()
	return labels[k3ktypes.W7_USER_MODE] == k3ktypes.W7_USER_MODE_NORMAL ||
		labels[k3ktypes.K3K_USER_MODE] == k3ktypes.W7_USER_MODE_NORMAL ||
		labels[k3ktypes.W7_ROLE] == k3ktypes.W7_USER_MODE_NORMAL
}

func configMapToPermission(cm *corev1.ConfigMap) *configv1alpha1.Permission {
	menu := []string{}
	_ = json.Unmarshal([]byte(cm.Data["menu"]), &menu)
	name := permissionservice.NormalizePermissionName(cm.Name)
	role := cm.Labels["w7.cc/role"]
	parentPermission := ""
	if name == permissionservice.FounderPermissionName {
		role = "founder"
	}
	if name == permissionservice.SuperPermissionName {
		role = "super"
	}
	if name == permissionservice.NormalPermissionName {
		role = "normal"
	}
	if cm.Labels["typemode"] != "in" {
		parentPermission = permissionservice.NormalPermissionName
		if role == "super" {
			parentPermission = permissionservice.SuperPermissionName
		}
		if role == "founder" {
			parentPermission = permissionservice.FounderPermissionName
		}
	}
	return &configv1alpha1.Permission{
		TypeMeta: v1.TypeMeta{
			APIVersion: "w7panel.w7.com/v1alpha1",
			Kind:       "Permission",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"typemode": cm.Labels["typemode"],
			},
		},
		Spec: configv1alpha1.PermissionSpec{
			Title:            cm.Annotations["title"],
			Type:             map[bool]string{true: "builtin", false: "custom"}[cm.Labels["typemode"] == "in"],
			Role:             role,
			ParentPermission: parentPermission,
			MenuRules:        menu,
			Features: configv1alpha1.PermissionFeatures{
				Debug:      cm.Data["debug"] == "true",
				Webshell:   cm.Data["webshell"] == "true",
				Fileeditor: cm.Data["fileeditor"] == "true",
			},
		},
	}
}
