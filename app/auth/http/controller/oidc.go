package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/oidc"
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

func (o Oidc) Discovery(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	oidc.SetLoadFunc(appgroup.AppGroupToOidcSecret)
	rep, err := server.Discovery(ctx, ctx.Request)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	// rep.Header 不需要处理
	headers := rep.Header
	for k, v := range headers {
		ctx.Header(k, v[0])
	}
	ctx.JSON(http.StatusOK, rep.Data)
}

func (o Oidc) RegisterClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() || !server.RegisterEnabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc registration disabled"})
		return
	}

	token := helper.GetToken(ctx)
	if !server.ValidateRegistrationAccessToken(token) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req oidcservice.DynamicClientRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}

	resp, err := server.RegisterDynamicClient(req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client_metadata", "error_description": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, resp)
}

func (o Oidc) GetClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() || !server.RegisterEnabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc registration disabled"})
		return
	}

	token := helper.GetToken(ctx)
	if !server.ValidateRegistrationAccessToken(token) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	resp, err := server.GetDynamicClient(ctx.Param("clientId"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not_found", "error_description": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (o Oidc) UpdateClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() || !server.RegisterEnabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc registration disabled"})
		return
	}

	token := helper.GetToken(ctx)
	if !server.ValidateRegistrationAccessToken(token) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req oidcservice.DynamicClientRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}

	resp, err := server.UpdateDynamicClient(ctx.Param("clientId"), req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_client_metadata", "error_description": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (o Oidc) DeleteClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() || !server.RegisterEnabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc registration disabled"})
		return
	}

	token := helper.GetToken(ctx)
	if !server.ValidateRegistrationAccessToken(token) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := server.DeleteDynamicClient(ctx.Param("clientId")); err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not_found", "error_description": err.Error()})
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (o Oidc) AuthorizeCode(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	// oidc.SetLoadFunc(appgroup.AppGroupToOidcSecret)
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

func (o Oidc) GetRedirectURI(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	// oidc.SetLoadFunc(appgroup.AppGroupToOidcSecret)
	// token := ctx.MustGet("k8s_token").(string)
	// k8sToken := k8s.NewK8sToken(token)
	// username, err := k8sToken.GetSaName()
	// if err != nil {
	// 	ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
	// 	return
	// }
	type authorizeCallbackResponse struct {
		CallbackURL string `json:"callbackUrl"`
	}

	type authorizeCallbackRequest struct {
		AuthRequestID string `json:"authRequestID" form:"authRequestID"`
		CallbackURL   string `json:"callbackUrl" form:"callbackUrl"`
	}

	var req authorizeCallbackRequest
	if err := ctx.ShouldBind(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}

	resp, err := server.BuildAuthorizationCallbackURL(ctx.Request.Context(), req.AuthRequestID)
	if err != nil {
		var oidcErr *zitadeloidc.Error
		if errors.As(err, &oidcErr) {
			ctx.JSON(http.StatusBadRequest, oidcErr)
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, authorizeCallbackResponse{
		CallbackURL: resp,
	})
}
