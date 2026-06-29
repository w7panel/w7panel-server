package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MicroAppSetting struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Spec 是微应用站点设置的具体配置。
	Spec MicroAppSettingSpec `json:"spec"`
}

type MicroAppSettingSpec struct {
	// Login 是登录设置。
	Login LoginSettings `json:"login"`
	// General 是通用站点设置。
	General GeneralSettings `json:"general"`
}

type LoginSettings struct {
	// LoginMode 表示登录方式。
	LoginMode string `json:"loginMode,omitempty"`
	// RegistrationEnabled 表示是否开启注册。
	RegistrationEnabled bool `json:"registrationEnabled,omitempty"`
	// IndexPage 表示登录前默认首页。
	IndexPage string `json:"indexPage,omitempty"`
	// ProtocolConfig 是注册协议与隐私协议配置。
	ProtocolConfig ProtocolConfig `json:"protocolConfig,omitempty"`
}

type ProtocolConfig struct {
	// UserAgreement 是用户注册协议内容引用的 ConfigMap。
	UserAgreement ConfigMapRef `json:"userAgreement,omitempty"`
	// PrivacyPolicy 是隐私协议内容引用的 ConfigMap。
	PrivacyPolicy ConfigMapRef `json:"privacyPolicy,omitempty"`
}

type GeneralSettings struct {
	// SiteName 是站点名称。
	SiteName string `json:"siteName,omitempty"`
	// SiteLogo 是站点 LOGO 引用的 ConfigMap。
	SiteLogo ConfigMapRef `json:"siteLogo,omitempty"`
	// SiteDescription 是站点描述。
	SiteDescription string `json:"siteDescription,omitempty"`
	// Filing 是备案相关设置。
	Filing FilingSettings `json:"filing,omitempty"`
	// ContactConfigs 是联系方式配置。
	ContactConfigs []ContactConfigSettings `json:"contactConfigs,omitempty"`
}

type FilingSettings struct {
	// ICP 是 ICP 备案信息。
	ICP string `json:"icp,omitempty"`
	// PublicSecurityNetworkFiling 是联网备案信息。
	PublicSecurityNetworkFiling string `json:"publicSecurityNetworkFiling,omitempty"`
	// ElectronicBusinessLicense 是电子执照信息。
	ElectronicBusinessLicense string `json:"electronicBusinessLicense,omitempty"`
	// ValueAddedTelecomBusinessLicense 是增值电信业务经营许可证信息。
	ValueAddedTelecomBusinessLicense string `json:"valueAddedTelecomBusinessLicense,omitempty"`
}

type ConfigMapRef struct {
	// Name 是被引用的 ConfigMap 名称。
	Name string `json:"name,omitempty"`
	// Key 是被引用的 ConfigMap 键名。
	Key string `json:"key,omitempty"`
}

type ContactConfigSettings struct {
	Type     string `json:"type,omitempty"`
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

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type MicroAppSettingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []MicroAppSetting `json:"items"`
}
