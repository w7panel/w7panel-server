package v1alpha1

import (
	cloudservice "github.com/w7corp/sdk-open-cloud-go/service"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var GVR = SchemeGroupVersion.WithResource("users")

type UserSpec struct {
	PasswordHash    string                             `json:"passwordHash,omitempty"`
	UserMode        string                             `json:"userMode,omitempty"`
	Role            string                             `json:"role,omitempty"`
	PermissionName  string                             `json:"permissionName,omitempty"`
	MenuRules       []string                           `json:"menuRules,omitempty"`
	APIRules        []configv1alpha1.PermissionAPIRule `json:"apiRules,omitempty"`
	Features        configv1alpha1.PermissionFeatures  `json:"features,omitempty"`
	DomainWhiteList []configv1alpha1.DomainWhiteItem   `json:"domainWhiteList,omitempty"`
	DemoUser        bool                               `json:"demoUser,omitempty"`
	ConsoleId       string                             `json:"consoleId,omitempty"`
	ConsoleOpenid   string                             `json:"consoleOpenid,omitempty"`
	ConsoleNickname string                             `json:"consoleNickname,omitempty"`
	W7Config        *W7Config                          `json:"w7Config,omitempty"`
	LoginTime       string                             `json:"loginTime,omitempty"`
}

type W7Config struct {
	ThirdpartyCDToken string                       `json:"thirdpartyCDToken,omitempty"`
	CDTokenExpireTime int                          `json:"cdTokenExpireTime,omitempty"`
	ClusterId         string                       `json:"clusterId,omitempty"`
	OfflineUrl        string                       `json:"offlineUrl,omitempty"`
	AccessToken       string                       `json:"accessToken,omitempty"`
	ExpireTime        int                          `json:"expireTime,omitempty"`
	ApiServerUrl      string                       `json:"apiServerUrl,omitempty"`
	UserInfo          *cloudservice.ResultUserinfo `json:"userInfo,omitempty"`
	License           string                       `json:"license,omitempty"`
	DebugValue        string                       `json:"debugValue,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:resource:path=users,scope=Cluster
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              UserSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []User `json:"items"`
}
