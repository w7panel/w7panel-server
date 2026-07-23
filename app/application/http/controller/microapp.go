package controller

import (

	// "github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s/microapp"
	"github.com/w7panel/w7panel/common/service/oidc"
	microappv1 "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type MicroApp struct {
	controller.Abstract
}

func (self MicroApp) List(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	list, err := microapp.ListTop(token)
	if err != nil {
		newList := &microappv1.MicroAppList{}
		self.JsonResponseWithoutError(http, newList)
		return
	}
	self.JsonResponseWithoutError(http, list)

}

func (self MicroApp) Info(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	microappObj, err := microapp.ListInfo(token, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithoutError(http, microappObj)
		return
	}
	configs := microappObj.Spec.ConfigV2.Props.RoleConfig
	if configs != nil {
		self.JsonResponseWithoutError(http, microappObj)
		return
	}
	for role, _ := range configs {
		config := microappObj.Spec.ConfigV2.Props.RoleConfig[role]
		props := replace.Replace(http, config.FrontendProps, role, microappObj)
		config.FrontendProps = props
	}

	self.JsonResponseWithoutError(http, microappObj)

}

func (self MicroApp) FrontProps(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	item, err := microapp.ListInfo(token, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	role := replace.GetRole()
	accessToken := ""
	server, err := oidc.GetServer()
	if err == nil && server != nil {
		accessToken, err = server.CreateDefaultAccessToken(server.ContextWithIssuer(http.Request.Context(), http.Request), replace.Name)
		if err != nil {
			accessToken = ""
		}
	}

	cloudAccessToken := ""
	if replace.GetConsoleOpenId() != "" {
		token, err := microapp.GetCloudAccessToken(replace.GetConsoleOpenId())
		if err == nil {
			cloudAccessToken = token
		}
	}

	self.JsonResponseWithoutError(http, map[string]string{
		// "url":               item.RoleServerUrl(role),
		"group":             item.Name,
		"userid":            replace.Name,
		"role":              role,
		"access_token":      accessToken,
		"openid":            replace.GetConsoleOpenId(),
		"nickname":          replace.GetNickName(),
		"cloud_uid":         replace.GetConsoleId(),
		"cloud_accesstoken": cloudAccessToken,
	})
}

func (self MicroApp) GlobalFrontProps(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)

	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	role := replace.GetRole()
	accessToken := ""
	server, err := oidc.GetServer()
	if err == nil && server != nil {
		accessToken, err = server.CreateDefaultAccessToken(server.ContextWithIssuer(http.Request.Context(), http.Request), replace.Name)
		if err != nil {
			accessToken = ""
		}
	}

	cloudAccessToken := ""
	if replace.GetConsoleOpenId() != "" {
		token, err := microapp.GetCloudAccessToken(replace.GetConsoleOpenId())
		if err == nil {
			cloudAccessToken = token
		}
	}

	self.JsonResponseWithoutError(http, map[string]string{
		// "url":               item.RoleServerUrl(role),
		"userid":            replace.Name,
		"role":              role,
		"access_token":      accessToken,
		"openid":            replace.GetConsoleOpenId(),
		"nickname":          replace.GetNickName(),
		"cloud_uid":         replace.GetConsoleId(),
		"cloud_accesstoken": cloudAccessToken,
	})
}
