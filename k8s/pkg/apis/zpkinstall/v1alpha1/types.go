// +kubebuilder:object:generate=true
// +groupName=w7panel.w7.com
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=zpki
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
type ZpkInstall struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ZpkInstallSpec   `json:"spec"`
	Status            ZpkInstallStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable; create a new installation task"
type ZpkInstallSpec struct {
	Namespace string `json:"namespace,omitempty"`
	// MaxRetries is the number of additional attempts after the first failed attempt.
	// +kubebuilder:validation:Minimum=0
	MaxRetries int32 `json:"maxRetries,omitempty"`
	// +kubebuilder:validation:MinLength=1
	RepoURL string `json:"repoUrl"`
	// +kubebuilder:validation:MinLength=1
	ReleaseName          string                    `json:"releaseName"`
	InstallOptions       []InstallOption           `json:"installOptions"`
	IngressHost          string                    `json:"ingressHost,omitempty"`
	IngressSeletorName   string                    `json:"ingressSeletorName,omitempty"`
	IngressClassName     string                    `json:"ingressClass,omitempty"`
	IngressForceHTTPS    bool                      `json:"ingressForceHttps,omitempty"`
	IsTrandition         bool                      `json:"isTrandition,omitempty"`
	ZipURL               string                    `json:"zipUrl,omitempty"`
	PanelURL             string                    `json:"panelUrl,omitempty"`
	Reinstall            bool                      `json:"reinstall,omitempty"`
	ThirdpartyCDTokenRef *corev1.SecretKeySelector `json:"thirdpartyCDTokenRef,omitempty"`
	PanelTokenRef        *corev1.SecretKeySelector `json:"panelTokenRef,omitempty"`
}
type InstallOption struct {
	Identifie                string               `json:"identifie,omitempty"`
	PvcName                  string               `json:"pvcname,omitempty"`
	DockerRegistry           DockerRegistry       `json:"registry,omitempty"`
	DockerRegistrySecretName string               `json:"dockerRegistrySecretName,omitempty"`
	Namespace                string               `json:"namespace,omitempty"`
	ReleaseName              string               `json:"releaseName,omitempty"` //安装name
	Suffix                   string               `json:"suffix,omitempty"`      //安装后缀 //releasename = root.Identifie+"-"+root.Suffix
	EnvKv                    []EnvKv              `json:"envkv,omitempty"`
	IngressSeletorName       string               `json:"ingressSelectorName,omitempty"` //IngressSelector前端选择的名称
	IngressHost              string               `json:"ingressHost,omitempty"`         //域名
	IngressClassName         string               `json:"ingressClassName,omitempty"`    //ingressclassname
	IngressForceHttps        bool                 `json:"ingressForceHttps,omitempty"`   //ingressclassname
	Replicas                 int32                `json:"replicas,omitempty"`            // 可选安装的数量为0
	IsChildApp               bool                 `json:"isChild,omitempty"`             //是否IsChildApp
	IsUpgrade                bool                 `json:"isUpgrade,omitempty"`           //是否更新模式
	Annotations              map[string]string    `json:"annotations,omitempty"`         //注解
	ParentReleaseName        string               `json:"parentReleaseName,omitempty"`   // 父节点发布名
	PreSubPath               map[string]string    `json:"preSubPath,omitempty"`          // 上次安装的子路径
	Cpu                      string               `json:"cpu,omitempty"`
	Memory                   string               `json:"memory,omitempty"`
	Volumes                  []corev1.Volume      `json:"volumes,omitempty"`
	VolumesMounts            []corev1.VolumeMount `json:"volumesMounts,omitempty"`
}
type EnvKv struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}
type DockerRegistry struct {
	Host      string `json:"host,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}
type ZpkInstallStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Unknown
	Phase       string       `json:"phase,omitempty"`
	ExecutionID string       `json:"executionId,omitempty"`
	StartedAt   *metav1.Time `json:"startedAt,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
	HeartbeatAt *metav1.Time `json:"heartbeatAt,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	Message     string       `json:"message,omitempty"`
	ReleaseName string       `json:"releaseName,omitempty"`
	Namespace   string       `json:"namespace,omitempty"`
	InstallID   string       `json:"installId,omitempty"`
	// RetryCount is the number of retries that have already been started.
	RetryCount int32 `json:"retryCount,omitempty"`
}

// +kubebuilder:object:root=true
type ZpkInstallList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ZpkInstall `json:"items"`
}
