package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

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
	StorageClassName string      `json:"storageClassName,omitempty"`
	Workload         Workload    `json:"workload,omitempty"`
	Resource         CvmResource `json:"resource,omitempty"` //可使用资源
	BaseOrder        *CvmOrder   `json:"baseOrder,omitempty"`
	ExpandOrder      *CvmOrder   `json:"expandOrder,omitempty"`
	RenewOrder       *CvmOrder   `json:"renewOrder,omitempty"`
	BaseResource     CvmResource `json:"baseResource,omitempty"` //首次购买资源 暂存在这个字段
	ExpireTime       string      `json:"expireTime,omitempty"`   //到期时间
	RecycleTime      string      `json:"expireTime,omitempty"`   //回收时间RECYCLE
	OverMode         string      `json:"overMode,omitempty"`     //资源状态 wait(等待检测) no-resource(无资源) success(检测通过)
}

type CvmOrder struct {
	OrderSn string `json:"orderSn"`
	Status  string `json:"status,omitempty"`
}

type Workload struct {
	metav1.TypeMeta `json:",inline"`
	PodTemplateName string `json:"podTemplateName"`
}

// 【微擎面板&集群云主机：云主机业务分离成独立应用】
// https://www.tapd.cn/tapd_fe/62789787/story/detail/1162789787001015242
type CvmStatus struct {
	Phase         string             `json:"phase,omitempty"`
	ReadyReplicas int32              `json:"readyReplicas,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CvmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Cvm `json:"items"`
}
