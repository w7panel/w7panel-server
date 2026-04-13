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

type CvmSpec struct {
	CPU              int64    `json:"cpu,omitempty"`
	Memory           int64    `json:"memory,omitempty"`
	Storage          int64    `json:"storage,omitempty"`
	Bandwidth        int64    `json:"bandwidth,omitempty"`
	StorageSize      int64    `json:"storageSize,omitempty"`
	StorageClassName string   `json:"storageClassName,omitempty"`
	Workload         Workload `json:"workload,omitempty"`
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
