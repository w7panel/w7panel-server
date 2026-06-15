package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	TokenTypeTemporary = "temporary"
	TokenTypePermanent = "permanent"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApiClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ApiClientSpec   `json:"spec"`
	Status            ApiClientStatus `json:"status,omitempty"`
}

type ApiClientSpec struct {
	Enabled               *bool  `json:"enabled,omitempty"`
	ClientID              string `json:"clientId,omitempty"`
	ClientName            string `json:"clientName,omitempty"`
	ClientSecret          string `json:"clientSecret,omitempty"`
	TokenType             string `json:"tokenType,omitempty"`
	TemporaryTokenMinutes *int64 `json:"temporaryTokenMinutes,omitempty"`
}

type ApiClientStatus struct {
	LastAccessedAt *metav1.Time `json:"lastAccessedAt,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ApiClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ApiClient `json:"items"`
}
