package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type CvmConsoleOrder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CvmConsoleOrderSpec `json:"spec"`
	Status            CvmStatus           `json:"status,omitempty"`
}

type CvmConsoleOrderSpec struct {
	Order   *CvmOrder `json:"order,omitempty"`
	CvmName string    `json:"cvmName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CvmConsoleOrderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CvmConsoleOrder `json:"items"`
}
