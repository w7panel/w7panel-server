package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OIDCClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OIDCClientSpec `json:"spec"`
}

type OIDCClientSpec struct {
	ClientID              string   `json:"clientId,omitempty"`
	ClientName            string   `json:"clientName,omitempty"`
	ClientSecret          string   `json:"clientSecret,omitempty"`
	RedirectURIs          []string `json:"redirectUris,omitempty"`
	AllowAnyRedirectURI   bool     `json:"allowAnyRedirectUri,omitempty"`
	Scopes                []string `json:"scopes,omitempty"`
	TokenEndpointAuthMode string   `json:"tokenEndpointAuthMethod,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OIDCClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OIDCClient `json:"items"`
}
