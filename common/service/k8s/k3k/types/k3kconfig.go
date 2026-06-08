package types

import corev1 "k8s.io/api/core/v1"

type K3kConfigSetting struct {
	AllowConsoleRegister bool   `json:"allowConsoleRegister"`
	DefaultPolicyName    string `json:"defaultPolicyName"`
}

func NewK3kConfig(allowConsoleRegister bool, defaultPolicyName string) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister: allowConsoleRegister,
		DefaultPolicyName:    defaultPolicyName,
	}
}

func NewK3kConfigBySecret(secret *corev1.Secret) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister: string(secret.Data["allowConsoleRegister"]) == "true",
		DefaultPolicyName:    string(secret.Data["defaultPolicyName"]),
	}
}
func NewK3kConfigByConfigmap(cm *corev1.ConfigMap) *K3kConfigSetting {
	return NewK3kConfigByData(cm.Data)
}

func NewK3kConfigByData(data map[string]string) *K3kConfigSetting {
	return &K3kConfigSetting{
		AllowConsoleRegister: data["allowConsoleRegister"] == "true",
		DefaultPolicyName:    data["defaultPolicyName"],
	}
}
