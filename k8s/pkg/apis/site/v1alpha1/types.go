package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TargetRef identifies a Kubernetes resource to patch with the registered site credentials.
type TargetRef struct {
	// APIVersion of the target resource (e.g. "apps/v1").
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind of the target resource (e.g. "Deployment").
	Kind string `json:"kind,omitempty"`
	// Namespace of the target resource.
	Namespace string `json:"namespace,omitempty"`
	// ContainerName is the container in the target resource to inject env vars into.
	ContainerName string `json:"containerName,omitempty"`
}

// SiteSpec defines the desired state of a ZPK site.
type SiteSpec struct {
	// Host is the domain name of the site.
	Host string `json:"host"`
	// SiteIdentifier is the unique identifier for the site.
	SiteIdentifier string `json:"siteIdentifier"`
	// UserName is the user who owns or manages this site.
	// +optional
	UserName string `json:"userName,omitempty"`
	// Target is the target Kubernetes resource to patch with the registered credentials.
	Target TargetRef `json:"target"`
	// AppId is the registered App ID returned from the panel.
	AppId string `json:"appId,omitempty"`
	// AppSecret is the registered App Secret returned from the panel.
	AppSecret string `json:"appSecret,omitempty"`
}

// SiteStatus defines the observed state of a ZPK site.
type SiteStatus struct {
	// Phase indicates the registration phase: Pending, Registered, or Failed.
	Phase string `json:"phase,omitempty"`
	// Conditions represents the latest available observations of the site's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// LastRegisteredAt is the timestamp of the last successful registration.
	LastRegisteredAt *metav1.Time `json:"lastRegisteredAt,omitempty"`
	// Message provides additional status description.
	Message string `json:"message,omitempty"`
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster

// Site is the Schema for the ZPK site registration API.
type Site struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SiteSpec   `json:"spec"`
	Status SiteStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SiteList contains a list of Site.
type SiteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Site `json:"items"`
}
