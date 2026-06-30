package controller

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service"
	auditservice "github.com/w7panel/w7panel/common/service/audit"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	saLogic "github.com/w7panel/w7panel/common/service/k8s/k3k/sa"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	userservice "github.com/w7panel/w7panel/common/service/user"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Auth struct {
	controller.Abstract
}

func (self Auth) Login(http *gin.Context) {
	self.login(http, true)
}

func (self Auth) LoginBySign(http *gin.Context) {
	self.login(http, false)
}

func (self Auth) login(http *gin.Context, verifyCaptcha bool) {
	type ParamsValidate struct {
		Username string `form:"username" json:"username" binding:"required"`
		Password string `form:"password" json:"password" binding:"required"`
		Point    string `form:"point" json:"point"`
		Key      string `form:"key" json:"key"`
	}
	loginMethod := "password"
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	if verifyCaptcha && facade.Config.GetBool("captcha.enabled") {
		if params.Point == "" || params.Key == "" {
			err := errors.New("验证码参数缺失")
			auditservice.RecordLoginFailure(http, params.Username, loginMethod, err)
			self.JsonResponseWithError(http, err, 500)
			return
		}
		err := helper.VerifyCaptcha(params.Point, params.Key, true)
		if err != nil {
			err2 := fmt.Errorf("验证码不正确")
			auditservice.RecordLoginFailure(http, params.Username, loginMethod, err2)
			self.JsonResponseWithError(http, err2, 500)
			return
		}
	}

	client := k8s.NewK8sClient()
	u, err := userservice.Login(http.Request.Context(), client.Sdk, params.Username, params.Password)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = errors.New("用户不存在")
			auditservice.RecordLoginFailure(http, params.Username, loginMethod, err)
			self.JsonResponseWithError(http, err, 500)
			return
		}
		auditservice.RecordLoginFailure(http, params.Username, loginMethod, err)
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.dologinUser(client.Sdk, u, http, loginMethod, "")
}

func (self Auth) Register(http *gin.Context) {
	type ParamsValidate struct {
		Username   string `form:"username" json:"username" binding:"required"`
		Password   string `form:"password" json:"password" binding:"required"`
		PolicyName string `form:"policyName" json:"policyName"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	permissionName := params.PolicyName
	if permissionName == "" {
		permissionName = "normal"
	}
	_, err := userservice.Create(http.Request.Context(), sdk, params.Username, params.Password, userservice.Spec{
		UserMode:       "normal",
		Role:           "normal",
		PermissionName: permissionName,
	})
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonSuccessResponse(http)
}

// http://127.0.0.1:9007/k8s/console/oauth?redirect_uri=http://127.0.0.1:9007/k8s/console/login
func (self Auth) ConsoleLogin(http *gin.Context) {
	type ParamsValidate struct {
		Code       string `form:"code" binding:"required"`
		PolicyName string `form:"policyName" json:"policyName"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	w7respo := config.NewW7ConfigRepository(sdk)
	client := console.DefaultClient(false)
	oclient := console.NewOauthClient(client, w7respo)
	accessToken, userInfo, err := oclient.GetUserInfo(params.Code)
	if err != nil {
		auditservice.RecordLoginFailure(http, "", "oauth", err)
		self.JsonResponseWithError(http, err, 500)
		return
	}

	w7config, err := w7respo.GetByConsoleId(strconv.Itoa(userInfo.UserId))
	if err != nil {
		//尝试注册用户
		sa, err := saLogic.DoRegister(sdk, types.NewConsoleOAuthAccessToken2(accessToken), userInfo, params.PolicyName)
		if err != nil {
			if !k8serrors.IsAlreadyExists(err) {
				auditservice.RecordLoginFailure(http, strconv.Itoa(userInfo.UserId), "oauth", err)
				self.JsonResponseWithError(http, err, 500)
				return
			}
		}
		_, err = oclient.BindUseAccessToken(sa.Name, accessToken)
		if err != nil {
			bindErr := err
			deleteErr := sdk.ClientSet.CoreV1().ServiceAccounts(sa.Namespace).Delete(sdk.Ctx, sa.Name, metav1.DeleteOptions{})
			if deleteErr != nil {
				slog.Error("删除serviceaccount失败", "err", deleteErr)
			}
			auditservice.RecordLoginFailure(http, sa.Name, "oauth", bindErr)
			self.JsonResponseWithError(http, bindErr, 500)
			return
		}

		self.dologin(sdk, sa, http, false, "")
		return
	}
	saName := w7config.Name
	_, err = oclient.BindUseAccessToken(saName, accessToken)
	if err != nil {
		auditservice.RecordLoginFailure(http, saName, "oauth", err)
		self.JsonResponseWithError(http, err, 500)
		return
	}
	sa, err := sdk.GetServiceAccount(sdk.GetNamespace(), saName)
	if err != nil {
		auditservice.RecordLoginFailure(http, saName, "oauth", err)
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.dologin(sdk, sa, http, true, "")

}

func (self Auth) dologin(sdk *k8s.Sdk, sa *corev1.ServiceAccount, http *gin.Context, updateK3kUser bool, ckmName string) {
	seconds := facade.Config.GetInt64("app.login_seconds")
	token, isK3kUser, err := k3k.LoginByServiceAccount(sdk, sa, seconds, updateK3kUser, ckmName)
	loginMethod := "password"
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = errors.New("用户不存在")
			auditservice.RecordLoginFailure(http, sa.Name, loginMethod, err)
			self.JsonResponseWithError(http, err, 500)
			return
		}
		auditservice.RecordLoginFailure(http, sa.Name, loginMethod, err)
		self.JsonResponseWithError(http, err, 500)
		return
	}
	rs := service.GetRefreshToken(sa.Name, ckmName)
	auditservice.RecordLoginSuccess(http, sa.Name, loginMethod, sa)
	self.JsonResponseWithoutError(http, gin.H{
		"token":         token,
		"expire":        time.Now().Add(time.Duration(seconds) * time.Second).Unix(),
		"isK3kUser":     isK3kUser, //废弃		废弃字段，后续删除
		"isClusterUser": isK3kUser,
		"refreshToken":  rs.Token,
	})
	return
}

func (self Auth) dologinUser(sdk *k8s.Sdk, u *userservice.User, http *gin.Context, loginMethod string, ckmName string) {
	seconds := facade.Config.GetInt64("app.login_seconds")
	token, err := userservice.SignToken(u, seconds)
	if err != nil {
		auditservice.RecordLoginFailure(http, u.Name, loginMethod, err)
		self.JsonResponseWithError(http, err, 500)
		return
	}
	rs := service.GetRefreshToken(u.Name, ckmName)
	auditservice.RecordLoginSuccessUser(http, u.Name, loginMethod, u)
	self.JsonResponseWithoutError(http, gin.H{
		"token":         token,
		"expire":        time.Now().Add(time.Duration(seconds) * time.Second).Unix(),
		"isK3kUser":     u.Spec.UserMode == "cluster",
		"isClusterUser": u.Spec.UserMode == "cluster",
		"refreshToken":  rs.Token,
	})
}

func (self Auth) RefreshToken2(http *gin.Context) {
	type ParamsValidate struct {
		Token string `form:"refreshToken" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	userName, cvmName, err := service.FindUsernameByToken(params.Token)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	u, err := userservice.Get(http.Request.Context(), sdk, userName)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.dologinUser(sdk, u, http, "refresh", cvmName)
}

func (self Auth) InitUser(http *gin.Context) {

	releaseName := facade.Config.GetString("app.helm_release_name")
	type ParamsValidate struct {
		Username string `form:"username" json:"username" binding:"required"`
		Password string `form:"password" json:"password" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	configMapName := releaseName + "-init-user"
	client := k8s.NewK8sClient()
	_, err := client.ClientSet.CoreV1().ConfigMaps(client.GetNamespace()).Get(http, configMapName, v1.GetOptions{})
	if err != nil {
		self.JsonResponseWithError(http, errors.New("已经初始化过用户"), 500)
		return
	}
	_, err = userservice.Create(http.Request.Context(), client.Sdk, params.Username, params.Password, userservice.Spec{
		UserMode:       "founder",
		Role:           "founder",
		PermissionName: "founder",
		Features: configv1alpha1.PermissionFeatures{
			Debug:      true,
			Webshell:   true,
			Fileeditor: true,
		},
	})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		self.JsonResponseWithError(http, errors.New("初始化用户失败"), 500)
		return
	}
	client.ClientSet.CoreV1().ConfigMaps(client.GetNamespace()).Delete(http, configMapName, metav1.DeleteOptions{})
	self.JsonSuccessResponse(http)

}

/*
*
获取用户信息
*/

/*
*
重置密码功能
*/
func (self Auth) ResetPassword(http *gin.Context) {

	type ParamsValidate struct {
		Username    string `form:"username" json:"username" binding:"required"`
		Password    string `form:"password" json:"password" binding:"required"`
		NewPassword string `form:"newPassword" json:"newPassword" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	sdk := k8s.NewK8sClient() //全局用户sdk

	// username := client.GetServiceAccountName()

	_, err := userservice.Login(http.Request.Context(), sdk.Sdk, params.Username, params.Password)
	if err != nil {
		self.JsonResponseWithError(http, fmt.Errorf("原始用户密码错误"), 500)
		return
	}
	err = userservice.ResetPassword(http.Request.Context(), sdk.Sdk, params.Username, params.NewPassword)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonResponseWithoutError(http, map[string]string{})
}

func (self Auth) ResetPasswordCurrent(http *gin.Context) {

	type ParamsValidate struct {
		Password    string `form:"password" json:"password"`
		NewPassword string `form:"newPassword" json:"newPassword" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	sdk := k8s.NewK8sClient() //全局用户sdk
	userName := http.GetString("username")
	if userName == "" {
		token := http.MustGet("k8s_token").(string)
		k8sToken := k8s.NewK8sToken(token)
		var err error
		userName, err = k8sToken.GetUserName()
		if err != nil {
			self.JsonResponseWithError(http, err, 500)
			return
		}
	}
	if userName == "" {
		self.JsonResponseWithError(http, fmt.Errorf("用户不存在"), 500)
		return
	}
	// username := client.GetServiceAccountName()
	u, err := userservice.Get(http.Request.Context(), sdk.Sdk, userName)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	if u.Spec.PasswordHash != "" {
		_, err := userservice.Login(http.Request.Context(), sdk.Sdk, userName, params.Password)
		if err != nil {
			self.JsonResponseWithError(http, fmt.Errorf("原始用户密码错误"), 500)
			return
		}
	}

	err = userservice.ResetPassword(http.Request.Context(), sdk.Sdk, userName, params.NewPassword)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonResponseWithoutError(http, map[string]string{})
}
