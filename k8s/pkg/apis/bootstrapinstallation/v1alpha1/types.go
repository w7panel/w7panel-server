package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	AnnotationInstallationOwner = "w7.cc/bootstrap-owner"
	InstallationFinalizer       = "w7.cc/installation-uninstall"
)

type ArtifactType string

const ArtifactTypeZPK ArtifactType = "ZPK"

type BootstrapPhase string

const (
	BootstrapPhasePending    BootstrapPhase = "Pending"
	BootstrapPhaseInstalling BootstrapPhase = "Installing"
	BootstrapPhaseReady      BootstrapPhase = "Ready"
	BootstrapPhaseFailed     BootstrapPhase = "Failed"
	BootstrapPhaseBlocked    BootstrapPhase = "Blocked"
)

type BootstrapStrategy struct {
	MaxConcurrent      int32           `json:"maxConcurrent,omitempty"`
	TimeoutPerArtifact metav1.Duration `json:"timeoutPerArtifact,omitempty"`
	MaxRetries         *int32          `json:"maxRetries,omitempty"`
}

type BootstrapInstallOptions struct {
	HelmValues map[string]string `json:"helmValues,omitempty"`
}

type ArtifactReference struct {
	Name      string       `json:"name"`
	Type      ArtifactType `json:"type,omitempty"`
	Identifie string       `json:"identifie"`
	Source    string       `json:"source"`
	// Version pins an exact target version. Empty or "latest" tracks the latest available version.
	Version string `json:"version,omitempty"`
}

type ArtifactTarget struct {
	ReleaseName string `json:"releaseName"`
	Namespace   string `json:"namespace"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=binstall
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type BootstrapInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BootstrapInstallationSpec   `json:"spec"`
	Status            BootstrapInstallationStatus `json:"status,omitempty"`
}

type BootstrapInstallationSpec struct {
	Strategy       BootstrapStrategy       `json:"strategy,omitempty"`
	Artifact       ArtifactReference       `json:"artifact"`
	Target         ArtifactTarget          `json:"target"`
	InstallOptions BootstrapInstallOptions `json:"installOptions,omitempty"`
}

type ArtifactAppGroupStatus struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type BootstrapInstallationStatus struct {
	Phase            BootstrapPhase         `json:"phase,omitempty"`
	InstalledVersion string                 `json:"installedVersion,omitempty"`
	AppGroup         ArtifactAppGroupStatus `json:"appGroup,omitempty"`
	OperationID      string                 `json:"operationID,omitempty"`
	RetryCount       int32                  `json:"retryCount,omitempty"`
	Message          string                 `json:"message,omitempty"`
	StartedAt        *metav1.Time           `json:"startedAt,omitempty"`
	CompletedAt      *metav1.Time           `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BootstrapInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootstrapInstallation `json:"items"`
}
