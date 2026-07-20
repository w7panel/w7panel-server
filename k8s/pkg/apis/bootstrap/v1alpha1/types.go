package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	LabelProfile      = "w7.cc/bootstrap-profile"
	LabelArtifact     = "w7.cc/bootstrap-artifact"
	ArtifactFinalizer = "bootstrap.w7.cc/artifact-uninstall"
)

type ArtifactType string

const (
	ArtifactTypeZPK ArtifactType = "ZPK"
)

type FailurePolicy string

const (
	FailurePolicyContinue FailurePolicy = "Continue"
	FailurePolicyStop     FailurePolicy = "Stop"
)

type RemovalPolicy string

const (
	RemovalPolicyUninstall RemovalPolicy = "Uninstall"
)

type ReinstallPolicy string

const (
	ReinstallPolicyRequired ReinstallPolicy = "Required"
)

type BootstrapPhase string

const (
	BootstrapPhasePending        BootstrapPhase = "Pending"
	BootstrapPhaseResolving      BootstrapPhase = "Resolving"
	BootstrapPhaseInstalling     BootstrapPhase = "Installing"
	BootstrapPhaseUpgrading      BootstrapPhase = "Upgrading"
	BootstrapPhaseUninstalling   BootstrapPhase = "Uninstalling"
	BootstrapPhaseReady          BootstrapPhase = "Ready"
	BootstrapPhaseFailed         BootstrapPhase = "Failed"
	BootstrapPhaseBlocked        BootstrapPhase = "Blocked"
	BootstrapPhaseSkipped        BootstrapPhase = "Skipped"
	BootstrapPhaseAheadOfProfile BootstrapPhase = "AheadOfProfile"
)

type ProfilePhase string

const (
	ProfilePhasePending     ProfilePhase = "Pending"
	ProfilePhaseProgressing ProfilePhase = "Progressing"
	ProfilePhaseReady       ProfilePhase = "Ready"
	ProfilePhaseDegraded    ProfilePhase = "Degraded"
	ProfilePhaseInvalid     ProfilePhase = "Invalid"
)

type BootstrapStrategy struct {
	MaxConcurrent      int32           `json:"maxConcurrent,omitempty"`
	TimeoutPerArtifact metav1.Duration `json:"timeoutPerArtifact,omitempty"`
	MaxRetries         int32           `json:"maxRetries,omitempty"`
}

type BootstrapDefaults struct {
	FailurePolicy   FailurePolicy   `json:"failurePolicy,omitempty"`
	RemovalPolicy   RemovalPolicy   `json:"removalPolicy,omitempty"`
	ReinstallPolicy ReinstallPolicy `json:"reinstallPolicy,omitempty"`
	AllowDowngrade  bool            `json:"allowDowngrade,omitempty"`
}

type BootstrapInstallOptions struct {
	HelmValues map[string]string `json:"helmValues,omitempty"`
}

type BootstrapArtifact struct {
	Name            string                  `json:"name"`
	Type            ArtifactType            `json:"type,omitempty"`
	Identifie       string                  `json:"identifie"`
	Source          string                  `json:"source"`
	ReleaseName     string                  `json:"releaseName"`
	Namespace       string                  `json:"namespace"`
	Version         string                  `json:"version,omitempty"`
	FailurePolicy   FailurePolicy           `json:"failurePolicy,omitempty"`
	RemovalPolicy   RemovalPolicy           `json:"removalPolicy,omitempty"`
	ReinstallPolicy ReinstallPolicy         `json:"reinstallPolicy,omitempty"`
	AllowDowngrade  *bool                   `json:"allowDowngrade,omitempty"`
	DependsOn       []string                `json:"dependsOn,omitempty"`
	InstallOptions  BootstrapInstallOptions `json:"installOptions,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=bprofile
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.revision`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type BootstrapProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BootstrapProfileSpec   `json:"spec"`
	Status            BootstrapProfileStatus `json:"status,omitempty"`
}

type BootstrapProfileSpec struct {
	Revision string            `json:"revision"`
	Strategy BootstrapStrategy `json:"strategy,omitempty"`
	Defaults BootstrapDefaults `json:"defaults,omitempty"`
	// +listType=map
	// +listMapKey=name
	Artifacts []BootstrapArtifact `json:"artifacts,omitempty"`
}

type BootstrapExpansionStatus struct {
	Total     int32 `json:"total,omitempty"`
	Processed int32 `json:"processed,omitempty"`
	Complete  bool  `json:"complete,omitempty"`
}

type BootstrapSummary struct {
	Total       int32 `json:"total,omitempty"`
	Ready       int32 `json:"ready,omitempty"`
	Progressing int32 `json:"progressing,omitempty"`
	Failed      int32 `json:"failed,omitempty"`
	Blocked     int32 `json:"blocked,omitempty"`
	Skipped     int32 `json:"skipped,omitempty"`
}

type BootstrapProfileStatus struct {
	ObservedRevision string                   `json:"observedRevision,omitempty"`
	Phase            ProfilePhase             `json:"phase,omitempty"`
	Expansion        BootstrapExpansionStatus `json:"expansion,omitempty"`
	Summary          BootstrapSummary         `json:"summary,omitempty"`
	Conditions       []metav1.Condition       `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type BootstrapProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BootstrapProfile `json:"items"`
}

type BootstrapProfileReference struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

type ArtifactReference struct {
	Name      string       `json:"name"`
	Type      ArtifactType `json:"type,omitempty"`
	Identifie string       `json:"identifie"`
	Source    string       `json:"source"`
	Version   string       `json:"version,omitempty"`
}

type ArtifactTarget struct {
	ReleaseName string `json:"releaseName"`
	Namespace   string `json:"namespace"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=ainstall
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Profile",type=string,JSONPath=`.spec.profileRef.name`
// +kubebuilder:printcolumn:name="Revision",type=string,JSONPath=`.spec.profileRevision`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type ArtifactInstallation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ArtifactInstallationSpec   `json:"spec"`
	Status            ArtifactInstallationStatus `json:"status,omitempty"`
}

type ArtifactInstallationSpec struct {
	ProfileRef      BootstrapProfileReference `json:"profileRef"`
	ProfileRevision string                    `json:"profileRevision"`
	Artifact        ArtifactReference         `json:"artifact"`
	Target          ArtifactTarget            `json:"target"`
	FailurePolicy   FailurePolicy             `json:"failurePolicy"`
	ReinstallPolicy ReinstallPolicy           `json:"reinstallPolicy"`
	RemovalPolicy   RemovalPolicy             `json:"removalPolicy"`
	AllowDowngrade  bool                      `json:"allowDowngrade,omitempty"`
	DependsOn       []string                  `json:"dependsOn,omitempty"`
	InstallOptions  BootstrapInstallOptions   `json:"installOptions,omitempty"`
}

type ArtifactAppGroupStatus struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type ArtifactInstallationStatus struct {
	ObservedProfileRevision string                 `json:"observedProfileRevision,omitempty"`
	Phase                   BootstrapPhase         `json:"phase,omitempty"`
	RequestedVersion        string                 `json:"requestedVersion,omitempty"`
	ResolvedVersion         string                 `json:"resolvedVersion,omitempty"`
	InstalledVersion        string                 `json:"installedVersion,omitempty"`
	AppGroup                ArtifactAppGroupStatus `json:"appGroup,omitempty"`
	OperationID             string                 `json:"operationID,omitempty"`
	RetryCount              int32                  `json:"retryCount,omitempty"`
	Message                 string                 `json:"message,omitempty"`
	StartedAt               *metav1.Time           `json:"startedAt,omitempty"`
	CompletedAt             *metav1.Time           `json:"completedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ArtifactInstallationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ArtifactInstallation `json:"items"`
}
