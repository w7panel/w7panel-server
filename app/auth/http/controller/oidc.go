package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	oidcservice "github.com/w7panel/w7panel/common/service/oidc"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
)

type Oidc struct {
	controller.Abstract
}

type authorizeCodeResponse struct {
	Code         string `json:"code"`
	State        string `json:"state,omitempty"`
	SessionState string `json:"session_state,omitempty"`
}

func (o Oidc) Handle(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	server.ServeHTTP(ctx.Writer, ctx.Request)
}

func (o Oidc) AuthorizeCode(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	token := ctx.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	username, err := k8sToken.GetSaName()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}

	var req oidcservice.DirectAuthorizeRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	req.Username = username

	resp, err := server.CreateDirectAuthorizationCode(ctx.Request.Context(), req)
	if err != nil {
		var oidcErr *zitadeloidc.Error
		if errors.As(err, &oidcErr) {
			ctx.JSON(http.StatusBadRequest, oidcErr)
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, authorizeCodeResponse{
		Code:         resp.Code,
		State:        resp.State,
		SessionState: resp.SessionState,
	})
}
