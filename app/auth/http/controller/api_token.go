package controller

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	k8sapiclient "github.com/w7panel/w7panel/common/service/k8s/apiclient"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	"github.com/w7panel/w7panel/common/service/panelauth"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type APIToken struct {
	controller.Abstract
}

func (self APIToken) Exchange(http *gin.Context) {
	type ParamsValidate struct {
		AppID     string `form:"appid" json:"appid" binding:"required"`
		AppSecret string `form:"appsecret" json:"appsecret" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	namespace := k8s.NewK8sClient().Sdk.GetNamespace()
	client, err := k8sapiclient.Authenticate(http.Request.Context(), namespace, params.AppID, params.AppSecret)
	if err != nil {
		if errors.Is(err, k8sapiclient.ErrInvalidCredentials) || errors.Is(err, k8sapiclient.ErrClientDisabled) {
			self.JsonResponseWithError(http, errors.New("appid or appsecret is invalid"), 401)
			return
		}
		slog.Error("exchange api token failed", "err", err)
		self.JsonResponseWithError(http, errors.New("exchange api token failed"), 500)
		return
	}

	ttl := time.Duration(k8sapiclient.TemporaryTokenSeconds(client.Spec.TemporaryTokenMinutes)) * time.Second
	if k8sapiclient.NormalizeTokenType(client.Spec.TokenType) == "permanent" {
		ttl = 24 * time.Hour
	}
	token, err := panelauth.Issue(panelauth.Principal{
		Username:       permissionservice.APIPermissionName,
		PermissionName: permissionservice.APIPermissionName,
		Role:           permissionservice.APIPermissionName,
		TokenUse:       panelauth.TokenUseExternalAPI,
	}, ttl)
	if err != nil {
		slog.Error("issue panel api token failed", "err", err)
		self.JsonResponseWithError(http, errors.New("exchange api token failed"), 500)
		return
	}
	http.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"token":     token,
			"tokenType": "panel",
			"expiresIn": int64(ttl.Seconds()),
		},
	})
}
