package types

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/w7panel/w7panel/common/service/config"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	userv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"
)

type K3kUser struct {
	*k3kUser
}

type k3kUser struct {
	*userv1alpha1.User
	Labels      map[string]string
	Annotations map[string]string
	ckmName     string
	// *k3kUserBase
	// *k3kUserOverSelling
}

func NewK3kUser(user *userv1alpha1.User) *K3kUser {
	if user.Labels == nil {
		user.Labels = map[string]string{}
	}
	if user.Annotations == nil {
		user.Annotations = map[string]string{}
	}
	u := &K3kUser{k3kUser: &k3kUser{
		User:   user,
		Labels: labelsFromUser(user),
	}}

	return u

}

func labelsFromUser(user *userv1alpha1.User) map[string]string {
	labels := map[string]string{}
	for k, v := range user.Labels {
		labels[k] = v
	}
	role := user.Spec.Role
	if role == "" {
		role = user.Spec.UserMode
	}
	if role == "" {
		role = W7_USER_MODE_NORMAL
	}
	labels[W7_USER_MODE] = role
	labels[W7_ROLE] = role
	labels[W7_DEMO_USER] = boolToString(user.Spec.DemoUser)
	if user.Spec.ConsoleId != "" {
		labels[W7_CONSOLE_ID] = user.Spec.ConsoleId
	}
	return labels
}

func (u *K3kUser) SyncSpecFromRuntime() {
	u.Spec.Role = u.GetRole()
	u.Spec.PermissionName = u.GetPermissionName()
	u.Spec.Features = configv1alpha1.PermissionFeatures{
		Debug:      u.Annotations[K3K_DEBUG] == "true",
		Webshell:   u.Annotations[W7_WEB_SHELL] == "true",
		Fileeditor: u.Annotations[W7_FILE_EDITTOR] == "true",
	}
	u.Spec.MenuRules = []string{}
	_ = json.Unmarshal([]byte(u.Annotations[W7_MENU]), &u.Spec.MenuRules)
	// u.Spec.DomainWhiteList = []configv1alpha1.DomainWhiteItem{}
	// _ = json.Unmarshal([]byte(u.GetDomainWhiteList()), &u.Spec.DomainWhiteList)

}

func mustJSONString(v interface{}) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func stringSliceJSON(v []string) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func domainWhiteListJSON(v []configv1alpha1.DomainWhiteItem) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func apiRulesJSON(v []configv1alpha1.PermissionAPIRule) string {
	api := make(map[string][]string, len(v))
	for _, rule := range v {
		if rule.Path == "" {
			continue
		}
		api[rule.Path] = append([]string(nil), rule.Method...)
	}
	data, _ := json.Marshal(api)
	return string(data)
}

func featuresFromPermission(features configv1alpha1.PermissionFeatures) map[string]string {
	return map[string]string{
		K3K_DEBUG:       boolToString(features.Debug),
		W7_WEB_SHELL:    boolToString(features.Webshell),
		W7_FILE_EDITTOR: boolToString(features.Fileeditor),
	}
}

func (u *K3kUser) ApplyPermission(menuName string, role string, menuRules []string, features configv1alpha1.PermissionFeatures, whiteList []configv1alpha1.DomainWhiteItem, apiRules []configv1alpha1.PermissionAPIRule) {
	u.Spec.PermissionName = menuName
	if role != "" {
		u.Spec.Role = role
	}
	u.Spec.MenuRules = append([]string(nil), menuRules...)
	// u.Annotations[W7_MENU] = stringSliceJSON(menuRules)
	u.Spec.DomainWhiteList = append([]configv1alpha1.DomainWhiteItem(nil), whiteList...)
	// u.Annotations[W7_DOMAIN_WHITE_LIST] = domainWhiteListJSON(whiteList)
	u.Spec.APIRules = append([]configv1alpha1.PermissionAPIRule(nil), apiRules...)
	// u.Annotations["w7.cc/api"] = apiRulesJSON(apiRules)
	// for k, v := range featuresFromPermission(features) {
	// 	u.Annotations[k] = v
	// }
	u.Spec.Features = features
}

func (u *k3kUser) GetK3kName() string {
	return u.Name
}

func (u *k3kUser) GetUserMode1() string {
	name, ok := u.Labels[K3K_USER_MODE]
	if ok {
		return name
	}
	return ""
}

func (u *k3kUser) GetK3kNamespace() string {
	// name, ok := u.Labels[K3K_NAMESPACE]
	// if ok {
	// 	return name
	// }
	return "k3k-" + u.GetName()
}

func (u *k3kUser) GetPermissionName() string {
	return u.Spec.PermissionName
}

func (u *k3kUser) GetRole() string {
	// if u.IsClusterUser() { //子集群用户默认是founder fix站点管理
	// 	return "founder"
	// }
	role, ok := u.Annotations[W7_ROLE]
	if ok {
		return role
	}
	if u.Labels[W7_USER_MODE] == "founder" {
		return "founder"
	}
	if u.Labels[W7_USER_MODE] == "normal" {
		return "normal"
	}
	return "normal"
}

func (u *k3kUser) GetTokenAud(cvmName string) []string {
	return []string{
		u.Name,
		u.GetRole(),
		u.Labels[W7_CONSOLE_ID],
		cvmName,
		u.GetK3kNamespace(),
		"https://kubernetes.default.svc.cluster.local",
		"k3s",
	}
}

func (u *k3kUser) GetConsoleId() string {
	return u.Spec.ConsoleId
}

func (u *k3kUser) GetConsoleOpenId() string {
	return u.Spec.ConsoleOpenid
}

func (u *k3kUser) GetNickName() string {
	return u.Spec.ConsoleNickname
}

func (u *k3kUser) GetDomainWhiteList1() string {
	value, ok := u.Annotations[W7_DOMAIN_WHITE_LIST]
	if !ok || value == "" || value == "null" {
		return "[]"
	}
	return value
}

// 自定义权限菜单
func (u *k3kUser) IsCustomPermission() bool {
	return u.Spec.PermissionName == ""
}

func (u *k3kUser) ReplaceW7Config(config *config.W7Config) {
	if config != nil && config.UserInfo != nil {
		u.Spec.ConsoleId = strconv.Itoa(config.UserInfo.UserId)
		u.Spec.ConsoleOpenid = config.UserInfo.OpenId
		u.Spec.ConsoleNickname = config.UserInfo.Nickname

	}
}

// 是否需要购买基础资源，

func (u *k3kUser) IsFounder() bool {
	return u.Spec.UserMode == W7_USER_MODE_FOUNDER
}

func (u *k3kUser) IsNormal() bool {
	return u.Spec.UserMode == W7_USER_MODE_NORMAL
}

// 是否必须超额检查，扩容不需要超额检查

func (u *k3kUser) SetLoginTime() {
	u.Spec.LoginTime = time.Now().Format("2006-01-02 15:04:05")
}

// 锁定退款订单

// 是否是演示用户
func (u *k3kUser) IsDemo() bool {
	return u.Spec.DemoUser
}

// 当前用户是否支持cvm 购买 续费 扩容
func (u *k3kUser) SupportCvm() bool {
	if u.Labels != nil && u.Labels[W7_CVM_USER] == "true" {
		return true
	}
	return false
}

// 是否是cvm请求用户 子集群请求用户
func (u *k3kUser) IsCkmReqUser() bool {
	return u.ckmName != ""
}

func (u *k3kUser) SetCkmName(name string) {
	u.ckmName = name
}

func (u *k3kUser) GetCvmName() string {
	return u.GetCkmName()
}
func (u *k3kUser) GetCkmName() string {
	return u.ckmName
}

func (u *k3kUser) ToArray1() map[string]string {

	return map[string]string{
		"uid": u.Name,
	}
}
