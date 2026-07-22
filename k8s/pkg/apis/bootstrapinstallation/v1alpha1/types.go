package v1alpha1

import (
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=binstall
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.profileRevision`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type BootstrapInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BootstrapInstallationSpec   `json:"spec"`
	Status            BootstrapInstallationStatus `json:"status,omitempty"`
}

type BootstrapInstallationSpec struct {
	ProfileRef      bootstrapv1.BootstrapProfileReference `json:"profileRef"`
	ProfileRevision string                                `json:"profileRevision"`
	Artifact        bootstrapv1.ArtifactReference         `json:"artifact"`
	Target          bootstrapv1.ArtifactTarget            `json:"target"`
	FailurePolicy   bootstrapv1.FailurePolicy             `json:"failurePolicy"`
	DependsOn       []string                              `json:"dependsOn,omitempty"`
	InstallOptions  bootstrapv1.BootstrapInstallOptions   `json:"installOptions,omitempty"`
}

type ArtifactAppGroupStatus struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type BootstrapInstallationStatus struct {
	ObservedProfileRevision string                     `json:"observedProfileRevision,omitempty"`
	Phase                   bootstrapv1.BootstrapPhase `json:"phase,omitempty"`
	InstalledVersion        string                     `json:"installedVersion,omitempty"`
	AppGroup                ArtifactAppGroupStatus     `json:"appGroup,omitempty"`
	OperationID             string                     `json:"operationID,omitempty"`
	RetryCount              int32                      `json:"retryCount,omitempty"`
	Message                 string                     `json:"message,omitempty"`
	StartedAt               *metav1.Time               `json:"startedAt,omitempty"`
	CompletedAt             *metav1.Time               `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BootstrapInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootstrapInstallation `json:"items"`
}
