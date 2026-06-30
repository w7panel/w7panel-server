package types

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/api/core/v1"
)

type K3kUser struct {
	*k3kUser
}

type k3kUser struct {
	*v1.ServiceAccount
	// *k3kUserBase
	// *k3kUserOverSelling
}

func NewK3kUser(sa *v1.ServiceAccount) *K3kUser {

	u := &K3kUser{k3kUser: &k3kUser{
		ServiceAccount: sa,
	}}

	return u

}
func (u *k3kUser) IsClusterReady() bool {
	return u.Annotations[K3K_JOB_STATUS] == K3K_STATUS_COMPLETE
}

func (u *k3kUser) IsClusterNew() bool {
	return u.Labels[K3K_CLUSTER_STATUS] == K3K_STATUS_USER_NEW
}

func (u *k3kUser) IsClusterCreating() bool {
	return u.Labels[K3K_CLUSTER_STATUS] == K3K_STATUS_USER_CREATING
}
func (u *k3kUser) IsClusterRecycle() bool {
	return u.Labels[K3K_CLUSTER_STATUS] == K3K_STATUS_USER_RECYCLE
}

func (u *k3kUser) IsClusterLabelReady() bool {
	return u.Labels[K3K_CLUSTER_STATUS] == K3K_STATUS_USER_READY
}

func (u *k3kUser) IsClusterUser() bool {
	return false
}

func (u *k3kUser) IsOldClusterUser() bool {
	return u.Labels[K3K_USER_MODE] == "cluster"
}

func (u *k3kUser) IsNormalUser() bool {
	return u.Labels[K3K_USER_MODE] == "normal"
}

// 测试不让agent 每次重建
func (u *k3kUser) SkipDaemonset() bool {
	return u.Labels["w7.cc/skip-ds"] == "true"
}

func (u *k3kUser) IsInitK3k() bool {
	name, ok := u.Labels[K3K_NAME]
	if ok {
		return name != ""
	}
	return false
}
func (u *k3kUser) GetK3kName() string {
	name, ok := u.Labels[K3K_NAME]
	if ok {
		return name
	}
	return u.Name
}

func (u *k3kUser) GetUserMode() string {
	name, ok := u.Labels[K3K_USER_MODE]
	if ok {
		return name
	}
	return ""
}
func (u *k3kUser) GetClusterMode() string {
	return "virtual"
	// name, ok := u.Annotations[K3K_CLUSTER_MODE]
	// if ok {
	// 	return name
	// }
	// return "unknown"
}

func (u *k3kUser) GetClusterPolicy() string {
	return "default"
	// name, ok := u.Annotations[K3K_CLUSTER_POLICY]
	// if ok {
	// 	return name
	// }
	// return ""
}

func (u *k3kUser) GetK3kNamespace() string {
	// name, ok := u.Labels[K3K_NAMESPACE]
	// if ok {
	// 	return name
	// }
	return "k3k-" + u.GetName()
}

func (u *k3kUser) GetK3kJobName() string {
	name, ok := u.Annotations[K3K_JOB_NAME]
	if ok {
		return name
	}
	return ""
}

func (u *k3kUser) GetK3kJobStatus() string {
	name, ok := u.Annotations[K3K_JOB_STATUS]
	if ok {
		return name
	}
	return K3K_STATUS_UNKNOW
}

func (u *k3kUser) GetAgentName() string {
	return helper.GetK3kAgentName(u.Name)
}

func (u *k3kUser) GetVirtualIngressServiceName(cvmName string) string {
	return helper.GetVirtualIngressServiceName(u.GetK3kNamespace(), cvmName)
}

func (u *k3kUser) GetApiServerHost() string {
	return helper.GetApiServerHost(u.GetK3kNamespace())
}

func (u *k3kUser) GetDefaultVolumeName() string {
	// if u.IsVirtual() {
	// 	return "default-volume"
	// }
	// return u.GetClusterServer0PvcName()
	return "default-volume"
}

// 是否维护模式
func (u *k3kUser) IsWeihu() bool {
	return u.Labels[W7_WH_MODE] == "true"
}

func (u *k3kUser) HasWeihuJob() bool {
	return u.Labels[W7_WH_JOB] != ""
}

func (u *k3kUser) SetWeihuJobName(name string) {
	u.Labels[W7_WH_JOB] = name
}
func (u *k3kUser) GetWeihuJobName() string {
	return u.Labels[W7_WH_JOB]
}
func (u *k3kUser) SetWHJobStatus(status string) {
	u.Labels[W7_WH_JOB_STATUS] = status
}
func (u *k3kUser) GetWHJobStatus() string {
	return u.Labels[W7_WH_JOB_STATUS]
}
func (u *k3kUser) GenerateWeihuJobName() string {
	return "k3k-" + u.Name + "-" + strings.ToLower(helper.RandomString(10))
}

func (u *k3kUser) SetWeihu(ok bool) {
	val := "false"
	if ok {
		val = "true"
	}
	u.Labels[W7_WH_MODE] = val
}

func (u *k3kUser) GetMenu() string {

	// if u.SupportCvm() && !u.IsCvmReqUser() {
	// 	whMenu := []string{"system-resource", "system-cloud"}
	// 	json, _ := json.Marshal(whMenu)
	// 	return string(json)
	// }
	// if u.IsWeihu() { //维护模式菜单
	// 	whMenu := []string{"cluster", "cluster/panel", "cluster/resource", "app", "app/apps", "app/apps/delete"}
	// 	json, _ := json.Marshal(whMenu)
	// 	return string(json)
	// }
	name, ok := u.Annotations[W7_MENU]
	if ok {
		// if console.IsFree() { //去掉 必须企业版的限制
		// 	name = strings.ReplaceAll(name, "system-manage", "system-manage-free") //替换为不存在的多租户管理菜单
		// }
		return name
	}
	return ""
}

func (u *k3kUser) GetMenuName() string {
	name, ok := u.Annotations[W7_MENU_NAME]
	if ok {
		if strings.HasPrefix(name, "k3k.permission.") {
			return strings.ReplaceAll(name, "k3k.permission.", "")
		}
		return name
	}
	return ""
}

func (u *k3kUser) GetQuotaName() string {
	name, ok := u.Annotations[W7_QUOTA_LIMIT_NAME]
	if ok {
		return name
	}
	return ""
}

func (u *k3kUser) GetCostName() string {
	name, ok := u.Annotations[W7_COST_NAME]
	if ok {
		return name
	}
	return ""
}

func (u *k3kUser) GetDebugMode() string {
	if !u.IsClusterUser() {
		return "true"
	}

	name, ok := u.Annotations[K3K_DEBUG]
	if ok {
		return name
	}
	return "false"
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

func (u *k3kUser) GetLockVersion() string {
	version, ok := u.Annotations[K3K_LOCK_VERSION]
	if ok {
		return version
	}
	return "1"
}

// 这是一个内存值，用于标记集群策略版本 auth.go 登录时候 实时查询
func (u *k3kUser) GetClusterPolicyVersion() string {
	version, ok := u.Annotations[K3K_CLUSTER_POLICY_VERSION]
	if ok {
		return version
	}
	return "1"
}

func (u *k3kUser) ToK3kConfig(cvmName string) *k8s.K3kConfig {
	return &k8s.K3kConfig{
		Name:      u.GetK3kName(),
		Namespace: u.GetK3kNamespace(),
		ApiServer: u.GetApiServerHost(),
		CvmName:   cvmName,
	}
}

// 获取资源回收阶段
func (u *k3kUser) GetResourceStatus() string {
	status, ok := u.Labels[K3K_CLUSTER_STATUS]
	if !ok {
		return K3K_STATUS_USER_NEW // 默认为有资源状态
	}
	return status
}

// 设置资源回收阶段
func (u *k3kUser) SetResourceStatus(status string) {
	if u.Labels == nil {
		u.Labels = make(map[string]string)
	}
	u.Labels[K3K_CLUSTER_STATUS] = status
}

// 检查是否过期

func (u *k3kUser) IsVirtual() bool {
	return u.GetClusterMode() == K3K_CLUSTER_MODE_VIRTUAL
}

func (u *k3kUser) IsShared() bool {
	return u.GetClusterMode() == K3K_CLUSTER_MODE_SHARED
}

func (u *k3kUser) GetClusterServer0PvcName() string {
	return "varlibrancherk3s-" + u.GetK3kNamespace() + "-server-0"
}

func (u *k3kUser) GetServer0Name() string {
	return u.GetK3kNamespace() + "-server-0"
}

func (u *k3kUser) GetServer0ContainerName() string {
	return u.GetK3kNamespace() + "-server"
}

func (u *k3kUser) GetConsoleId() string {
	return u.Labels["w7.cc/console-id"]
}

func (u *k3kUser) GetConsoleOpenId() string {
	return u.Annotations["w7.cc/console-openid"]
}

func (u *k3kUser) GetDomainWhiteList() string {
	value, ok := u.Annotations[W7_DOMAIN_WHITE_LIST]
	if !ok || value == "" || value == "null" {
		return "[]"
	}
	return value
}

// 自定义权限菜单
func (u *k3kUser) IsCustomPermission() bool {
	return u.Annotations["w7.cc/menu-name"] == ""
}

// 自定义配额
func (u *k3kUser) IsCustomQuota() bool {
	return u.Annotations["w7.cc/quota-limit-name"] == ""
}

func (u *k3kUser) IsCustomCost() bool {
	return u.Annotations["w7.cc/cost-name"] == ""
}

func (u *k3kUser) ReplaceMenu(menu *v1.ConfigMap) {
	// u.Annotations[W7_MENU_NAME] = menu.Name
	u.Annotations[K3K_DEBUG] = menu.Data["debug"]
	u.Annotations[W7_MENU] = menu.Data["menu"]
	u.Annotations[W7_WEB_SHELL] = menu.Data["webshell"]
	u.Annotations[W7_FILE_EDITTOR] = menu.Data["fileeditor"]
	u.Annotations[W7_DOMAIN_WHITE_LIST] = "[]"
	if value := menu.Annotations[W7_DOMAIN_WHITE_LIST]; value != "" && value != "null" {
		u.Annotations[W7_DOMAIN_WHITE_LIST] = value
	}
	if menu.Labels[W7_ROLE] != "" {
		u.Labels[W7_ROLE] = menu.Labels[W7_ROLE]
	}
}

func (u *k3kUser) ReplaceW7Config(config *config.W7Config) {
	if config != nil && config.UserInfo != nil {
		u.Labels[W7_CONSOLE_ID] = strconv.Itoa(config.UserInfo.UserId)
		u.Annotations["w7.cc/console-nickname"] = config.UserInfo.Nickname
		u.Annotations["w7.cc/console-openid"] = config.UserInfo.OpenId
		// u.Annotations[W7_USER_MODE] = config.UserInfo.UserMode
		// return 0, fmt.Errorf("user cost is not empty")
	}
}

// 是否需要购买基础资源，

func (u *k3kUser) IsFounder() bool {
	return u.Labels[W7_USER_MODE] == W7_USER_MODE_FOUNDER
}

func (u *k3kUser) IsNormal() bool {
	return u.Labels[W7_USER_MODE] == W7_USER_MODE_NORMAL
}

func (u *k3kUser) Pause() {
	u.Annotations[W7_PAUSE] = "true"
}

func (u *k3kUser) UnPause() {
	u.Annotations[W7_PAUSE] = "false"
}

func (u *k3kUser) IsPause() bool {
	return u.Annotations[W7_PAUSE] == "true"
}

// 是否必须超额检查，扩容不需要超额检查

func (u *k3kUser) SetLoginTime() {
	u.Annotations[W7_LOGIN_TIME] = time.Now().Format(time.DateTime)
}

// 锁定退款订单

// 是否是演示用户
func (u *k3kUser) IsDemo() bool {
	val, ok := os.LookupEnv("DEMO_USER")
	if ok && val == "true" {
		return true
	}
	if u.Labels != nil && u.Labels[W7_DEMO_USER] == "true" {
		return true
	}
	return false
}

// 当前用户是否支持cvm 购买 续费 扩容
func (u *k3kUser) SupportCvm() bool {
	if u.Labels != nil && u.Labels[W7_CVM_USER] == "true" {
		return true
	}
	return false
}

// 是否是cvm请求用户 子集群请求用户
func (u *k3kUser) IsCvmReqUser() bool {
	if u.Annotations == nil {
		return false
	}
	return u.Annotations[W7_CVM_NAME] != ""
}

func (u *k3kUser) SetCvmName(name string) {
	if u.Annotations == nil {
		u.Annotations = make(map[string]string)
	}
	u.Annotations[W7_CVM_NAME] = name
}

func (u *k3kUser) GetCvmName() string {
	return u.GetCkmName()
}
func (u *k3kUser) GetCkmName() string {
	if u.Annotations == nil {
		return ""
	}
	return u.Annotations[W7_CVM_NAME]
}
func (u *k3kUser) GetNickName() string {
	return u.Annotations["w7.cc/console-nickname"]
}

func (u *k3kUser) ToArray() map[string]string {

	return map[string]string{
		"uid": u.Name,
	}
}
