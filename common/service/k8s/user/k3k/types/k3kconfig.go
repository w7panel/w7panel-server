package types

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type K3kConfigSetting struct {
	AllowConsoleRegister  bool   `json:"allowConsoleRegister"`
	DefaultPermissionName string `json:"defaultPermissionName"`
}

func NewK3kConfig(allowConsoleRegister bool, defaultPermissionName string) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister:  allowConsoleRegister,
		DefaultPermissionName: defaultPermissionName,
	}
}

func NewK3kConfigBySecret(secret *corev1.Secret) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister: string(secret.Data["allowConsoleRegister"]) == "true",
		DefaultPermissionName: defaultPermissionName(map[string]string{
			"defaultPermissionName": string(secret.Data["defaultPermissionName"]),
			"defaultPolicyName":     string(secret.Data["defaultPolicyName"]),
		}),
	}
}
func NewK3kConfigByConfigmap(cm *corev1.ConfigMap) *K3kConfigSetting {
	return NewK3kConfigByData(cm.Data)
}

func NewK3kConfigByData(data map[string]string) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister:  data["allowConsoleRegister"] == "true",
		DefaultPermissionName: defaultPermissionName(data),
	}
}

func defaultPermissionName(data map[string]string) string {
	if data["defaultPermissionName"] != "" {
		return normalizePermissionName(data["defaultPermissionName"])
	}
	return normalizePermissionName(data["defaultPolicyName"])
}

func normalizePermissionName(name string) string {
	switch name {
	case "k3k.permission.founder", "permission.founder":
		return "founder"
	case "k3k.permission.super", "permission.super":
		return "super"
	case "k3k.permission.normal", "permission.normal":
		return "normal"
	case "k3k.permission.api", "permission.api":
		return "api"
	default:
		return strings.TrimPrefix(strings.TrimPrefix(name, "k3k.permission."), "permission.")
	}
}
