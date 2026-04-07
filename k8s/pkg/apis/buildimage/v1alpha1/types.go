package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type BuildImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BuildImageSpec `json:"spec"`
}

type BuildImageSpec struct {
	TaskID    string `json:"taskId"`
	Namespace string `json:"namespace"`
	Source    struct {
		DownloadURL    string `json:"downloadUrl"`
		DockerfilePath string `json:"dockerfilePath"`
	} `json:"source"`
	TargetImage struct {
		Address string `json:"address"`
		Auth    struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"auth"`
	} `json:"targetImage"`
	NotifyURL string `json:"notifyUrl"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BuildImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []BuildImage `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BuildImageStatus struct {
	Status     string             `json:"status"`
	Reason     string             `json:"reason"`
	Contitions []metav1.Condition `json:"conditions"`
	JobName    string             `json:"jobName"`
}
