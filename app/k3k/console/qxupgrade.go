package console

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
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
}

func (QxUpgrade) handleConfigmaps(configmaps *corev1.ConfigMapList, sigClient sigclient.Client) {
	for _, configmap := range configmaps.Items {
		if configmap.Name == "k3k.permission.founder" {
			// configmap.Labels["w7.cc/role"] = "founder"
			configmap.Labels["typemode"] = "in"
			err := sigClient.Update(context.Background(), &configmap)
			if err != nil {
				slog.Error("Failed to update configmap", "error", err)
			}
			continue
		}
		if configmap.Labels != nil {
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
}

func (QxUpgrade) migratePermissions(sdk *k8s.Sdk, configmaps *corev1.ConfigMapList) {
	gvr := schema.GroupVersionResource{Group: "w7panel.w7.com", Version: "v1alpha1", Resource: "permissions"}
	for _, cm := range configmaps.Items {
		if cm.Name == "k3k.permission.tech" {
			continue
		}
		p := configMapToPermission(&cm)
		obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(p)
		if err != nil {
			slog.Error("Failed to convert permission", "error", err, "name", cm.Name)
			continue
		}
		_, err = sdk.DynamicClient().Resource(gvr).Create(context.Background(), &unstructured.Unstructured{Object: obj}, v1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			slog.Error("Failed to create permission", "error", err, "name", cm.Name)
		}
	}
}

func configMapToPermission(cm *corev1.ConfigMap) *configv1alpha1.Permission {
	menu := []string{}
	_ = json.Unmarshal([]byte(cm.Data["menu"]), &menu)
	name := cm.Name
	role := cm.Labels["w7.cc/role"]
	parentPermission := ""
	if cm.Name == permissionservice.FounderPermissionName {
		role = "founder"
	}
	if cm.Name == "k3k.permission.super" || cm.Name == permissionservice.AdminPermissionName {
		name = permissionservice.AdminPermissionName
		role = "admin"
	}
	if cm.Name == permissionservice.NormalPermissionName {
		role = "normal"
	}
	if cm.Labels["typemode"] != "in" {
		parentPermission = permissionservice.NormalPermissionName
		if role == "admin" || role == "super" {
			parentPermission = permissionservice.AdminPermissionName
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
			Menu:             menu,
			Features: configv1alpha1.PermissionFeatures{
				Debug:      cm.Data["debug"] == "true",
				Webshell:   cm.Data["webshell"] == "true",
				Fileeditor: cm.Data["fileeditor"] == "true",
			},
		},
	}
}
