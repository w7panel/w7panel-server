package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// CostPricing 资源单价配置
type CostPricing struct {
	// Cpu CPU 单价
	Cpu       float64 `json:"cpu"`
	// Memory 内存单价
	Memory    float64 `json:"memory"`
	// Storage 存储单价
	Storage   float64 `json:"storage"`
	// Bandwidth 带宽单价
	Bandwidth float64 `json:"bandwidth"`
}

// CostQuota 默认资源配额
type CostQuota struct {
	// StorageClassName 默认存储类
	StorageClassName string            `json:"storageClassName"`
	// Hard 配额上限
	Hard             map[string]intstr.IntOrString `json:"hard"`
}

// CostConfigSpec 费用配置
type CostConfigSpec struct {
	// Pricing 资源单价配置
	Pricing       CostPricing `json:"pricing"`
	// PackageConfig 套餐配置 JSON 字符串，保持与原 ConfigMap.data.packageConfig 一致
	PackageConfig string      `json:"packageConfig"`
	// Quota 默认资源配额
	Quota         CostQuota   `json:"quota"`
}

// CostConfigStatus 费用配置状态
type CostConfigStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=costconfigs,scope=Namespaced,shortName=ccfg
// +kubebuilder:printcolumn:name="CPUPrice",type=number,JSONPath=`.spec.pricing.cpu`
// +kubebuilder:printcolumn:name="MemPrice",type=number,JSONPath=`.spec.pricing.memory`
// +kubebuilder:printcolumn:name="StoragePrice",type=number,JSONPath=`.spec.pricing.storage`
// +kubebuilder:printcolumn:name="BandwidthPrice",type=number,JSONPath=`.spec.pricing.bandwidth`
// +kubebuilder:printcolumn:name="StorageClass",type=string,JSONPath=`.spec.quota.storageClassName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type CostConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CostConfigSpec   `json:"spec"`
	Status CostConfigStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CostConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CostConfig `json:"items"`
}
