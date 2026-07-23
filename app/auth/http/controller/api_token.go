package controller

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	k8sapiclient "github.com/w7panel/w7panel/common/service/k8s/apiclient"
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
	result, err := k8sapiclient.ExchangeToken(http.Request.Context(), namespace, params.AppID, params.AppSecret)
	if err != nil {
		if errors.Is(err, k8sapiclient.ErrInvalidCredentials) || errors.Is(err, k8sapiclient.ErrClientDisabled) {
			self.JsonResponseWithError(http, errors.New("appid or appsecret is invalid"), 401)
			return
		}
		slog.Error("exchange api token failed", "err", err)
		self.JsonResponseWithError(http, errors.New("exchange api token failed"), 500)
		return
	}

	http.JSON(200, gin.H{
		"code": 200,
		"data": gin.H{
			"token":     result.Token,
			"tokenType": result.TokenType,
			"expiresIn": result.ExpiresIn,
		},
	})
}
