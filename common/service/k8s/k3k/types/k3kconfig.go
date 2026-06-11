package types

import corev1 "k8s.io/api/core/v1"

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
		return data["defaultPermissionName"]
	}
	return data["defaultPolicyName"]
}
