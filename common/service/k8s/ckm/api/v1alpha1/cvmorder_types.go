package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type CkmConsoleOrder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CkmConsoleOrderSpec `json:"spec"`
	Status            CkmStatus           `json:"status,omitempty"`
}

type CkmConsoleOrderSpec struct {
	Order    *CkmOrder `json:"order,omitempty"`
	CkmName  string    `json:"ckmName"`
	CostName string    `json:"costName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CkmConsoleOrderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []CkmConsoleOrder `json:"items"`
}
