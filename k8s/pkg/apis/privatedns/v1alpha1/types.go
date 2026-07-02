package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PrivateDNSRecord defines a DNS record in a private DNS zone.
type PrivateDNSRecord struct {
	// ID is an optional stable record identifier. If empty, the controller derives one from record content.
	// +optional
	ID string `json:"id,omitempty"`
	// Name is the host label. Use "@" or empty for the zone apex.
	Name string `json:"name,omitempty"`
	// Type is the DNS record type. Supported values match the CoreDNS service layer: A, AAAA, CNAME, MX, NS, TXT.
	Type string `json:"type"`
	// Value is the DNS record value.
	Value string `json:"value"`
	// TTL is the record TTL in seconds. Defaults to the CoreDNS service default when omitted.
	// +optional
	TTL int `json:"ttl,omitempty"`
	// MXPriority is used by MX records. Defaults to the CoreDNS service default when omitted.
	// +optional
	MXPriority int `json:"mxPriority,omitempty"`
}

// PrivateDNSSpec defines the desired state of a private DNS zone.
type PrivateDNSSpec struct {
	// Domain is the DNS zone domain, for example "example.com".
	Domain string `json:"domain"`
	// Records is the complete desired record set for the zone.
	// +optional
	Records []PrivateDNSRecord `json:"records,omitempty"`
}

// PrivateDNSStatus defines the observed state of a private DNS zone.
type PrivateDNSStatus struct {
	// Phase is Pending, Ready, or Failed.
	Phase string `json:"phase,omitempty"`
	// Message provides additional status detail.
	Message string `json:"message,omitempty"`
	// ObservedGeneration is the last reconciled metadata generation.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// RecordCount is the number of records applied to the zone.
	RecordCount int `json:"recordCount,omitempty"`
	// Conditions represents the latest observations of the resource state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope=Cluster,path=privatedns,singular=privatedns
// +kubebuilder:subresource:status

// PrivateDNS is the Schema for managing CoreDNS private zones.
type PrivateDNS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PrivateDNSSpec   `json:"spec"`
	Status PrivateDNSStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PrivateDNSList contains a list of PrivateDNS.
type PrivateDNSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []PrivateDNS `json:"items"`
}
