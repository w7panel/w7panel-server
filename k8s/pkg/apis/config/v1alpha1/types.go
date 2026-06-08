package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigSpec struct {
	Data map[string]string `json:"data"`
}

type LicenseSpec struct {
	AppId         string `json:"appId"`
	AppSecret     string `json:"appSecret"`
	FounderSaName string `json:"founderSaName"`
	License       string `json:"license,omitempty"`
}

type OverSellingConfigSpec struct {
	CPU          int32 `json:"cpu"`
	Memory       int32 `json:"memory"`
	Storage      int32 `json:"storage"`
	BandWidth    int32 `json:"bandwidth"`
	BandWidthNum int32 `json:"bandwidthNum"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=k3kconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3kConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3kConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []K3kConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=k3sconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3sConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3sConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []K3sConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=licenses,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type License struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LicenseSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LicenseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []License `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=oversellingconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OverSellingConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OverSellingConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OverSellingConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OverSellingConfig `json:"items"`
}
