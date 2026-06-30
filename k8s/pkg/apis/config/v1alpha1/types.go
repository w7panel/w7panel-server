package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConfigSpec struct {
	Data map[string]string `json:"data"`
}

type LicenseSpec struct {
	AppId         string `json:"appId"`
	AppSecret     string `json:"appSecret"`
	FounderSaName string `json:"founderSaName"`
	License       string `json:"license,omitempty"`
}

type OverSellingConfigSpec struct {
	CPU          int32 `json:"cpu"`
	Memory       int32 `json:"memory"`
	Storage      int32 `json:"storage"`
	BandWidth    int32 `json:"bandwidth"`
	BandWidthNum int32 `json:"bandwidthNum"`
}

type FilingConfigSpec struct {
	IcpNumber string `json:"icpnumber,omitempty"`
	Number    string `json:"number,omitempty"`
	Location  string `json:"location,omitempty"`
	License   string `json:"license,omitempty"`
	Tbol      string `json:"tbol,omitempty"`
}

type DomainParseConfigSpec struct {
	Type  string   `json:"type"`
	IPs   []string `json:"ips,omitempty"`
	Cname string   `json:"cname,omitempty"`
}

type ContactConfigSpec struct {
	Type     string `json:"type"`
	Link     string `json:"link,omitempty"`
	Text     string `json:"text,omitempty"`
	Name     string `json:"name,omitempty"`
	ShowName bool   `json:"showName,omitempty"`
	SelIcon  string `json:"selicon,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Qrcode   string `json:"qrcode,omitempty"`
	Style    string `json:"style,omitempty"`
	Index    int32  `json:"index,omitempty"`
}

type PermissionFeatures struct {
	Debug      bool `json:"debug,omitempty"`
	Webshell   bool `json:"webshell,omitempty"`
	Fileeditor bool `json:"fileeditor,omitempty"`
}

type PermissionAPIRule struct {
	Path   string   `json:"path,omitempty"`
	Method []string `json:"method,omitempty"`
}

type DomainWhiteItem struct {
	Prefix       string `json:"prefix,omitempty"`
	Domain       string `json:"domain,omitempty"`
	PrefixRandom bool   `json:"prefixRandom,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
	Enable       *bool  `json:"enable,omitempty"`
}

type PermissionSpec struct {
	Title            string              `json:"title,omitempty"`
	Type             string              `json:"type,omitempty"`
	Role             string              `json:"role,omitempty"`
	ParentPermission string              `json:"parentPermission,omitempty"`
	MenuRules        []string            `json:"menuRules,omitempty"`
	APIRules         []PermissionAPIRule `json:"apiRules,omitempty"`
	Features         PermissionFeatures  `json:"features,omitempty"`
	DomainWhiteList  []DomainWhiteItem   `json:"domainWhiteList,omitempty"`
	RBACRules        []rbacv1.PolicyRule `json:"rbacRules,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=k3kconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3kConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3kConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []K3kConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=k3sconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3sConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type K3sConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []K3sConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=licenses,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type License struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              LicenseSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type LicenseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []License `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=oversellingconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OverSellingConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OverSellingConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OverSellingConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []OverSellingConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=filingconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FilingConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              FilingConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type FilingConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []FilingConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=domainparseconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DomainParseConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DomainParseConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type DomainParseConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []DomainParseConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=contactconfigs,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ContactConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ContactConfigSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ContactConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ContactConfig `json:"items"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=permissions,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type Permission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PermissionSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Permission `json:"items"`
}
