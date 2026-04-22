package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type Cvm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CvmSpec   `json:"spec"`
	Status            CvmStatus `json:"status,omitempty"`
}

type CvmResource struct {
	CPU       int64 `json:"cpu,omitempty"`
	Memory    int64 `json:"memory,omitempty"`
	Storage   int64 `json:"storage,omitempty"`
	Bandwidth int64 `json:"bandwidth,omitempty"`
}

type CvmSpec struct {
	StorageClassName  string       `json:"storageClassName,omitempty"`
	Workload          Workload     `json:"workload,omitempty"`
	DesiredResource   *CvmResource `json:"desiredResource,omitempty"`   // 目标资源
	PurchasedResource *CvmResource `json:"purchasedResource,omitempty"` // 已购买资源，待检测后生效
	ProvisionMode     string       `json:"provisionMode,omitempty"`     // 开通模式 order-required/admin-bypass
	BaseOrder         *CvmOrder    `json:"baseOrder,omitempty"`         // 首次购买 基础订单
	ExpandOrder       *CvmOrder    `json:"expandOrder,omitempty"`       // 扩容订单
	RenewOrder        *CvmOrder    `json:"renewOrder,omitempty"`        // 续费订单 延长到期时间
	ExpireTime        string       `json:"expireTime,omitempty"`        // 到期时间
	RecycleTime       string       `json:"recycleTime,omitempty"`       // 回收时间RECYCLE
	Rescue            bool         `json:"rescue,omitempty"`            // 是否救援模式
}

// 购买信息
type CvmOrder struct {
	OrderSn  string       `json:"orderSn"`
	Status   string       `json:"status,omitempty"`
	Resource *CvmResource `json:"resource,omitempty"`
	Hour     int          `json:"hour,omitempty"`
}

type Workload struct {
	metav1.TypeMeta `json:",inline"`
	TemplateName    string `json:"templateName"`
}

// 【微擎面板&集群云主机：云主机业务分离成独立应用】
// https://www.tapd.cn/tapd_fe/62789787/story/detail/1162789787001015242
type CvmStatus struct {
	Phase              string             `json:"phase,omitempty"`
	ReadyReplicas      int32              `json:"readyReplicas,omitempty"`
	EffectiveResource  *CvmResource       `json:"effectiveResource,omitempty"`
	CapacityCheckState string             `json:"capacityCheckState,omitempty"` // wait/no-resource/success
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CvmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Cvm `json:"items"`
}
