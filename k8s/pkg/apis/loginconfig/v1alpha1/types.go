package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LoginConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LoginConfigSpec `json:"spec"`
}

// LoginConfigSpec keeps cluster-global provider settings together so new login
// providers do not require another global configuration resource.
type LoginConfigSpec struct {
	Providers []LoginProvider `json:"providers"`
}

type LoginProvider struct {
	Type         string   `json:"type"`
	Enabled      bool     `json:"enabled"`
	Immutable    bool     `json:"immutable,omitempty"`
	DiscoveryURL string   `json:"discoveryUrl,omitempty"`
	ClientID     string   `json:"clientId,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LoginConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []LoginConfig `json:"items"`
}
