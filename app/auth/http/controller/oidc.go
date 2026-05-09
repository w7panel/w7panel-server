package controller

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	oidcservice "github.com/w7panel/w7panel/common/service/oidc"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Oidc struct {
	controller.Abstract
}

func (o Oidc) Handle(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	server.ServeHTTP(ctx.Writer, ctx.Request)
}

func (o Oidc) AuthorizeLogin(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.JsonResponseWithError(ctx, errors.New("oidc disabled"), http.StatusNotFound)
		return
	}
	requestID := strings.TrimSpace(ctx.Query("authRequestID"))
	if requestID == "" {
		requestID = strings.TrimSpace(ctx.PostForm("id"))
	}
	if requestID == "" {
		o.JsonResponseWithError(ctx, errors.New("missing authRequestID"), http.StatusBadRequest)
		return
	}

	if ctx.Request.Method == http.MethodGet {
		sessionID := server.GetSessionID(ctx.Request)
		if session, ok := server.FindSession(sessionID); ok {
			if err := server.CompleteAuthRequest(requestID, session.Username); err == nil {
				ctx.Redirect(http.StatusFound, server.CallbackURL(ctx.Request.Context(), requestID))
				return
			}
		}
		o.renderLogin(ctx, requestID, "")
		return
	}

	username := strings.TrimSpace(ctx.PostForm("username"))
	password := ctx.PostForm("password")
	if err := server.Login(ctx.Request.Context(), requestID, username, password); err != nil {
		o.renderLogin(ctx, requestID, err.Error())
		return
	}

	session := server.CreateSession(username)
	server.SetSessionCookie(ctx.Writer, ctx.Request, session)
	ctx.Redirect(http.StatusFound, server.CallbackURL(ctx.Request.Context(), requestID))
}

func (o Oidc) RegisterClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}
	if !server.RegisterEnabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "dynamic client registration disabled")
		return
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	if !server.ValidateRegistrationAccessToken(bearer) {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", "invalid registration access token")
		return
	}
	var req oidcservice.DynamicClientRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	resp, err := server.RegisterDynamicClient(req)
	if err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, resp)
}

func (o Oidc) GetClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}
	if !server.RegisterEnabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "dynamic client registration disabled")
		return
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	if !server.ValidateRegistrationAccessToken(bearer) {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", "invalid registration access token")
		return
	}
	resp, err := server.GetDynamicClient(ctx.Param("clientId"))
	if err != nil {
		o.oauthError(ctx, http.StatusNotFound, "invalid_client", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (o Oidc) UpdateClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}
	if !server.RegisterEnabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "dynamic client registration disabled")
		return
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	if !server.ValidateRegistrationAccessToken(bearer) {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", "invalid registration access token")
		return
	}
	var req oidcservice.DynamicClientRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	resp, err := server.UpdateDynamicClient(ctx.Param("clientId"), req)
	if err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (o Oidc) DeleteClient(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}
	if !server.RegisterEnabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "dynamic client registration disabled")
		return
	}
	bearer := strings.TrimSpace(strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer "))
	if !server.ValidateRegistrationAccessToken(bearer) {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", "invalid registration access token")
		return
	}
	if err := server.DeleteDynamicClient(ctx.Param("clientId")); err != nil {
		o.oauthError(ctx, http.StatusNotFound, "invalid_client", err.Error())
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (o Oidc) oauthError(ctx *gin.Context, status int, code string, description string) {
	ctx.JSON(status, gin.H{
		"error":             code,
		"error_description": description,
	})
}

func (o Oidc) renderLogin(ctx *gin.Context, requestID string, errMsg string) {
	form := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OIDC Login</title>
  <style>
    body { font-family: sans-serif; background: #f4f7fb; margin: 0; padding: 32px; color: #1f2937; }
    .card { max-width: 420px; margin: 0 auto; background: #fff; border-radius: 12px; box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08); padding: 24px; }
    h1 { margin-top: 0; font-size: 22px; }
    label { display: block; margin: 12px 0 6px; font-size: 14px; }
    input { width: 100%; box-sizing: border-box; padding: 10px 12px; border: 1px solid #d1d5db; border-radius: 8px; }
    button { width: 100%; margin-top: 18px; border: 0; border-radius: 8px; padding: 12px; background: #111827; color: #fff; font-size: 14px; cursor: pointer; }
    .error { color: #b91c1c; margin: 8px 0 0; font-size: 14px; }
    .meta { color: #6b7280; font-size: 13px; margin-bottom: 16px; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Sign in to w7panel</h1>
    <div class="meta">authRequestID={{.id}}</div>
    {{if .error}}<div class="error">{{.error}}</div>{{end}}
    <form method="post" action="/panel-api/v1/oidc/authorize/login">
      <input type="hidden" name="id" value="{{.id}}">
      <label>Username</label>
      <input name="username" autocomplete="username">
      <label>Password</label>
      <input type="password" name="password" autocomplete="current-password">
      <button type="submit">Continue</button>
    </form>
  </div>
</body>
</html>`
	tpl := template.Must(template.New("login").Parse(form))
	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(ctx.Writer, map[string]string{
		"id":    requestID,
		"error": errMsg,
	}); err != nil {
		fmt.Fprint(ctx.Writer, template.HTMLEscapeString(err.Error()))
	}
}
