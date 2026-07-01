package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/microapp"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type K3k struct {
	controller.Abstract
}

/*
*
 */
func (self K3k) Info(http *gin.Context) {
	if username := http.GetString("username"); username != "" {
		sdk := k8s.NewK8sClient().Sdk
		if u, err := userservice.Get(http.Request.Context(), sdk, username); err == nil {
			p, _ := userservice.ResolvePermission(http.Request.Context(), sdk, u)
			menu := ""
			api := ""
			if p != nil {
				menu = mustJSON(permissionservice.MenuRules(p))
				api = mustJSON(permissionservice.APIMap(p))
			}
			if len(u.Spec.MenuRules) > 0 {
				menu = mustJSON(u.Spec.MenuRules)
			}
			if len(u.Spec.APIRules) > 0 {
				api = mustJSON(permissionservice.APIRulesToMap(u.Spec.APIRules))
			}
			result := map[string]string{
				k3ktypes.K3K_USER_MODE:        u.Spec.UserMode,
				"w7.cc/username":              u.Name,
				k3ktypes.K3K_NAME:             u.Name,
				k3ktypes.K3K_NAMESPACE:        sdk.GetNamespace(),
				k3ktypes.K3K_DEBUG:            boolString(u.Spec.Features.Debug || (p != nil && p.Spec.Features.Debug)),
				k3ktypes.W7_FILE_EDITTOR:      boolString(u.Spec.Features.Fileeditor || (p != nil && p.Spec.Features.Fileeditor)),
				k3ktypes.W7_WEB_SHELL:         boolString(u.Spec.Features.Webshell || (p != nil && p.Spec.Features.Webshell)),
				k3ktypes.W7_MENU:              menu,
				"w7.cc/api":                   api,
				k3ktypes.W7_DOMAIN_WHITE_LIST: mustJSON(u.Spec.DomainWhiteList),
				k3ktypes.W7_DEMO_USER:         boolString(u.Spec.DemoUser),
				k3ktypes.W7_ROLE:              u.Spec.Role,
				"w7.cc/has-password":          boolString(u.Spec.PasswordHash != ""),
			}
			self.JsonResponseWithoutError(http, result)
			return
		}
	}

	token := http.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	// rootSdk := k8s.NewK8sClient().Sdk
	if user != nil {
		result := map[string]string{}
		self.JsonResponseWithoutError(http, result)
		return
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func mustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Error("json marshal error", "error", err)
		return "[]"
	}
	if data == nil || string(data) == "null" {
		return "[]"
	}
	return string(data)
}

func (self K3k) ReInitCluster(http *gin.Context) {
	// token := http.MustGet("k8s_token").(string)
	// user, err := k3k.TokenToK3kUser(token)
	// if err != nil {
	// 	self.JsonResponseWithServerError(http, err)
	// 	return
	// }
	// err = k3k.InitCluster(k8s.NewK8sClient().Sdk, user)
	// if err != nil {
	// 	self.JsonResponseWithServerError(http, err)
	// 	return
	// }
	// self.JsonSuccessResponse(http)
	// return
	self.JsonSuccessResponse(http) //不需要初始化集群
}

/*
*

	云端注册需要 转化token
*/
func (self K3k) LoginCvm(http *gin.Context) {

	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	name := http.Param("name")
	namespace := http.Param("namespace")

	if k8sToken.IsK3kCluster() {
		namespace = k8sToken.GetNamespace()
	}
	// user, err := k3k.TokenToK3kUser(token)
	// if err != nil {
	// 	self.JsonResponseWithServerError(http, err)
	// 	return
	// }
	client := k8s.NewK8sClient().Sdk

	cvm, err := k3k.TokenToCkm(http, token, namespace, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	seconds := facade.Config.GetInt64("app.login_seconds")
	sa, err := client.Login2(cvm.GetK3kName(), "", false)
	if err != nil {
		err2 := fmt.Errorf("用户名密码不正确")
		self.JsonResponseWithError(http, err2, 500)
		return
	}
	token, isK3kUser, err := k3k.LoginByServiceAccount(client, sa, seconds, true, cvm.Name)
	if err != nil {
		err2 := fmt.Errorf("用户名密码不正确")
		self.JsonResponseWithError(http, err2, 500)
		return
	}
	rs := service.GetRefreshToken(sa.Name, cvm.Name)
	self.JsonResponseWithoutError(http, gin.H{
		"token":        token,
		"expire":       time.Now().Add(time.Duration(seconds) * time.Second).Unix(),
		"isK3kUser":    isK3kUser,
		"refreshToken": rs.Token,
	})
}

func (self K3k) Login(http *gin.Context) {

	type ParamsValidate struct {
		K3kUserName string `form:"k3kUserName" validate:"required"`
		CvmName     string `form:"cvmName" validate:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	token := http.MustGet("k8s_token").(string)
	client, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if !user.IsFounder() && user.Name != "w7panel" {
		self.JsonResponseWithServerError(http, errors.New("非创始人无法操作"))
		return
	}
	seconds := facade.Config.GetInt64("app.login_seconds")
	sa, err := client.Login2(params.K3kUserName, "", false)
	if err != nil {
		err2 := fmt.Errorf("用户名密码不正确")
		self.JsonResponseWithError(http, err2, 500)
		return
	}
	token, isK3kUser, err := k3k.LoginByServiceAccount(client, sa, seconds, true, params.CvmName)
	if err != nil {
		err2 := fmt.Errorf("用户名密码不正确")
		self.JsonResponseWithError(http, err2, 500)
		return
	}
	rs := service.GetRefreshToken(sa.Name, "")
	self.JsonResponseWithoutError(http, gin.H{
		"token":        token,
		"expire":       time.Now().Add(time.Duration(seconds) * time.Second).Unix(),
		"isK3kUser":    isK3kUser,
		"refreshToken": rs.Token,
	})
}

func (self K3k) SyncIngress(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncIngress(&params)
	if err != nil {
		slog.Error("同步ingress失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncConfigmap(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncConfigmap(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	if params.K3kMode == "virtual" && params.VirtualName == "registries" {
		// 删除集群pod 重启集群
		// time.AfterFunc(time.Second*10, func() {
		// 	rootsdk := k8s.NewK8sClient().Sdk
		// 	statefulsets, err := rootsdk.ClientSet.AppsV1().StatefulSets(params.K3kNamespace).List(context.Background(), metav1.ListOptions{LabelSelector: "cluster"})
		// 	if err != nil {
		// 		self.JsonResponseWithServerError(http, err)
		// 		return
		// 	}
		// 	for _, statefulset := range statefulsets.Items {
		// 		// 创建patch来更新Pod template annotations
		// 		patchData := map[string]interface{}{
		// 			"spec": map[string]interface{}{
		// 				"template": map[string]interface{}{
		// 					"metadata": map[string]interface{}{
		// 						"annotations": map[string]interface{}{
		// 							"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
		// 						},
		// 					},
		// 				},
		// 			},
		// 		}

		// 		// 将patch数据转换为JSON
		// 		patchBytes, err := json.Marshal(patchData)
		// 		if err != nil {
		// 			slog.Error("Failed to marshal patch data", "error", err)
		// 			continue
		// 		}

		// 		// 使用strategic merge patch更新StatefulSet
		// 		_, err = rootsdk.ClientSet.AppsV1().StatefulSets(params.K3kNamespace).Patch(
		// 			context.Background(),
		// 			statefulset.Name,
		// 			types.StrategicMergePatchType,
		// 			patchBytes,
		// 			metav1.PatchOptions{},
		// 		)
		// 		if err != nil {
		// 			slog.Error("Failed to patch StatefulSet", "name", statefulset.Name, "error", err)
		// 			continue
		// 		}

		// 		slog.Info("Successfully restarted StatefulSet", "name", statefulset.Name)
		// 	}
		// })

	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncMcpBridge(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncMcpBridge(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncSecret(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	slog.Error("同步secret")
	err := k3k.SyncSecret(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncDownStatic(http *gin.Context) {
	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	slog.Error("同步down-static")
	appgroup.DownStaticGo(params.VirtualNamespace, params.VirtualName, "")
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncMicroApp(http *gin.Context) {
	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	// slog.Error("同步SyncMicroApp")
	microapp.Sync(params.K3kName, params.K3kNamespace, params.CkmName)
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncSite(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncSite(&params)
	if err != nil {
		slog.Error("同步site失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}
