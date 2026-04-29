package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/w7panel/w7panel/common/service/k8s/microapp"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type K3k struct {
	controller.Abstract
}

/*
*
 */
func (self K3k) Info(http *gin.Context) {

	token := http.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	// rootSdk := k8s.NewK8sClient().Sdk
	if user != nil {
		result := user.ToArray()
		self.JsonResponseWithoutError(http, result)
		return
	}
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

func (self K3k) DomainWhiteList(http *gin.Context) {

	sdk := k8s.NewK8sClient().Sdk //白名单强制使用root sdk
	http.Request.Header.Del("Authorization")
	sdk.Proxy(http.Request, http.Writer)
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
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	client := k8s.NewK8sClient().Sdk

	cvm, err := k3k.GetCvm(http, client, namespace, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	seconds := facade.Config.GetInt64("app.login_seconds")
	sa, err := client.Login2(user.Name, "", false)
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
	rs := service.GetRefreshToken(sa.Name)
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
	rs := service.GetRefreshToken(sa.Name)
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
	microapp.Sync(params.K3kName, params.K3kNamespace, params.CvmName)
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) ResizeSysStorage(http *gin.Context) {
	type ParamsValidate struct {
		Size int `form:"size"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	token := http.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	resizeTo := resource.MustParse(fmt.Sprintf("%dGi", params.Size))
	err = k3k.Resize(user, resizeTo)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return

}

func (self K3k) IdcResource(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	client, err := sdk.ToSigClient()
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}
	list := &v1.ConfigMapList{}
	err = client.List(http, list)
	if err != nil {
		self.JsonResponseWithoutError(http, list)
		return
	}
	result := types.Params{}
	for _, v := range list.Items {
		if (v.Labels != nil) && (v.Labels["w7.cc/showInShop"] != "true") {
			continue
		}

		policy := types.NewK3kCostConfigMap(&v)
		params, err := policy.ToPackageItemsParams(true)
		if err != nil {
			slog.Warn("idc resource err", "error", err)
			continue
		}
		result = append(result, params...)
	}

	self.JsonResponseWithoutError(http, result)

}
