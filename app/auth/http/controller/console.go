package controller

import (
	// "archive/zip"

	"errors"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

// 控制台相关接口
type Console struct {
	controller.Abstract
}

func (self Console) Redirect(http *gin.Context) {
	// file := http.Param("file")
	// self.FileDownload(http, file)
	type ParamsValidate struct {
		RedirectUrl string `form:"redirect_uri" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	client := console.DefaultClient(false)
	redirectUrl, err := client.OauthService.GetLoginUrl(params.RedirectUrl)
	if err.Errno != 0 {
		self.JsonResponseWithError(http, err.ToError(), 500)
		return
	}
	redirectUrl = redirectUrl + "&confirm_account=1"
	//判断是否accept json 请求
	if http.GetHeader("Accept") == "application/json" {
		self.JsonResponseWithoutError(http, map[string]interface{}{"url": redirectUrl})
	}

	http.Redirect(302, redirectUrl)
}

// http://172.16.1.126:9007/k8s/console/oauth?redirect_uri=http://172.16.1.126:9007/k8s/console/bind
// 绑定控制台
func (self Console) BindConsole(gin *gin.Context) {

	type ParamsValidate struct {
		Code string `form:"code" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(gin, &params) {
		return
	}
	tokenstr := gin.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(tokenstr)
	saName, err := k8sToken.GetUserName()
	if err != nil {
		self.JsonResponseWithError(gin, err, 500)
		return
	}
	// sdk, err := k8s.NewK8sClient().Channel(tokenstr) // 子集群的sdk 需要使用root sdk
	// if err != nil {
	// 	self.JsonResponseWithServerError(gin, err)
	// 	return
	// }
	sdk := k8s.NewK8sClient().Sdk
	respo := config.NewW7ConfigRepository(sdk)
	client := console.DefaultClient(helper.IsDebug())
	oclient := console.NewOauthClient(client, respo)
	userInfo, err := oclient.Bind(params.Code, saName)
	if err != nil {
		self.JsonResponseWithError(gin, err, 500)
		return
	}

	gin.Writer.Header().Set("uid", fmt.Sprintf("%d", userInfo.UserId))
	slog.Info("绑定控制台成功")
	self.JsonSuccessResponse(gin)
}

func (self Console) Info(gin *gin.Context) {

	tokenstr := gin.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(tokenstr)
	saName, err := k8sToken.GetUserName()
	if err != nil {
		self.JsonResponseWithError(gin, err, 500)
		return
	}
	// sdk, err := k8s.NewK8sClient().Channel(tokenstr)
	// if err != nil {
	// 	self.JsonResponseWithServerError(gin, err)
	// 	return
	// }
	sdk := k8s.NewK8sClientInner()
	w7respo := config.NewW7ConfigRepository(sdk)
	w7config, err := w7respo.Get(saName)
	if err != nil {
		w7config = config.NewEmptyConfig()
	}
	licenseClient, err := console.NewDefaultLicenseClient()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	license, err := licenseClient.GetLicense()
	if (err == nil) && (license != nil) {
		w7config.License = license.License
	}

	self.JsonResponseWithoutError(gin, w7config.ToArray())
}

func (self Console) JsCloudCode(gin *gin.Context) {
	componentAppId := gin.Query("componentAppId")
	if componentAppId == "" {
		componentAppId = gin.PostForm("componentAppId")
	}
	if componentAppId == "" {
		self.JsonResponseWithError(gin, errors.New("componentAppId is required"), 400)
		return
	}

	token := gin.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	openId := user.GetConsoleOpenId()
	if openId == "" {
		self.JsonResponseWithError(gin, errors.New("openid is empty"), 400)
		return
	}

	code, err := console.OpenIdToCloudCode(openId, componentAppId)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	self.JsonResponseWithoutError(gin, code)
}

// 绑定集群到控制台交付系统集群
func (self Console) RegisterToConsole(gin *gin.Context) {

	type ParamsValidate struct {
		OfflineUrl   string `form:"offline_url" binding:"required"`
		ApiServerUrl string `form:"api_server_url"`
	}
	params := ParamsValidate{}
	if !self.Validate(gin, &params) {
		return
	}
	if helper.IsLocalMock() {
		params.OfflineUrl = "http://218.23.2.48:9090/"
	}

	token := gin.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	rootSdk := k8s.NewK8sClient().Sdk
	// sdk, err := k8s.NewK8sClient().Channel(token)
	// if err != nil {
	// 	self.JsonResponseWithServerError(gin, err)
	// 	return
	// }
	sdk := k8s.NewK8sClient().Sdk
	saName, err := k8sToken.GetUserName()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}

	respo := config.NewW7ConfigRepository(sdk)
	w7config, err := respo.Get(saName)
	if err != nil {
		w7config = config.NewEmptyConfig()
	}
	apiServerUrl := w7config.ApiServerUrl
	if params.ApiServerUrl != "" {
		apiServerUrl = params.ApiServerUrl
	}
	if apiServerUrl == "" {
		apiServerUrl, err = sdk.GetApiServerUrl()
		if err != nil {
			self.JsonResponseWithServerError(gin, fmt.Errorf("获取apiserver地址失败"))
			return
		}
	}
	//kubeconfig 需要rootsdk
	kubeconfig, err := rootSdk.ToKubeconfig(apiServerUrl)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	consoleClient := console.NewClusterClient(respo, sdk, kubeconfig)
	consoleClient.SetOfflineUrl(params.OfflineUrl)

	err = consoleClient.RegisterUseCdToken(k8sToken.IsK3kCluster(), saName)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	// 暂时不需要创建license站点了，先用旧版的逻辑处理
	// consoleClient.CreateLicenseSite(k8sToken.IsK3kCluster(), saName)

	self.JsonSuccessResponse(gin)

}

func (self Console) ImportCert(gin *gin.Context) {
	type ParamsValidate struct {
		Cert string `form:"cert" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(gin, &params) {
		return
	}
	token := gin.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	// sdk, err := k8s.NewK8sClient().Channel(token)
	// if err != nil {
	// 	self.JsonResponseWithServerError(gin, err)
	// 	return
	// }
	// sdk := k8s.NewK8sClient().Sdk
	saName, err := k8sToken.GetUserName()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	licenseClient, err := console.NewDefaultLicenseClient()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	err = licenseClient.ImportCert([]byte(params.Cert), saName)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}

	self.JsonSuccessResponse(gin)
}

func (self Console) ImportCertConsole(gin *gin.Context) {
	type ParamsValidate struct {
		LicenseId string `form:"licenseId" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(gin, &params) {
		return
	}
	token := gin.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	// sdk, err := k8s.NewK8sClient().Channel(token)
	// if err != nil {
	// 	self.JsonResponseWithServerError(gin, err)
	// 	return
	// }
	// sdk := k8s.NewK8sClient().Sdk
	saName, err := k8sToken.GetUserName()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	err = console.VerifyLicenseId(params.LicenseId, saName)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}

	self.JsonSuccessResponse(gin)
}

func (self Console) VerifyCert(gin *gin.Context) {
	// 重新验证license
	console.VerifyDefaultLicense(true)
	self.JsonSuccessResponse(gin)
}

func (self Console) Proxy(gin *gin.Context) {

	sdkclient, err := console.NewDefaultSdkClient()
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	var path = gin.Param("path")
	if path == "" {
		path = "/"
	}
	proxy, err := sdkclient.Proxy(path, "")
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	defer func() {
		//golang issue 23643
		if r := recover(); r != nil {
			slog.Error("客户端已断开连接", "error", r)
		}
	}()
	proxy.ServeHTTP(gin.Writer, gin.Request)
}
