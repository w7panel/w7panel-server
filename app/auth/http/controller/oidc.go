package controller

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	oidcservice "github.com/w7panel/w7panel/common/service/oidc"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Oidc struct {
	controller.Abstract
}

func (o Oidc) Discovery(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	ctx.JSON(http.StatusOK, server.Discovery(ctx.Request))
}

func (o Oidc) JWKS(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	ctx.JSON(http.StatusOK, server.JWKS())
}

func (o Oidc) Authorize(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.JsonResponseWithError(ctx, errors.New("oidc disabled"), http.StatusNotFound)
		return
	}

	req, client, ok := o.parseAuthorizeRequest(ctx, server)
	if !ok {
		return
	}

	sessionID := server.GetSessionID(ctx.Request)
	session, hasSession := server.FindSession(sessionID)
	if req.Prompt == "login" {
		hasSession = false
	}
	if !hasSession {
		if req.Prompt == "none" {
			o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "login_required", "")
			return
		}
		o.renderLogin(ctx, req, "")
		return
	}

	o.finishAuthorize(ctx, server, client, req, session.Username)
}

func (o Oidc) AuthorizeLogin(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.JsonResponseWithError(ctx, errors.New("oidc disabled"), http.StatusNotFound)
		return
	}

	req, client, ok := o.parseAuthorizeRequest(ctx, server)
	if !ok {
		return
	}

	username := strings.TrimSpace(ctx.PostForm("username"))
	password := ctx.PostForm("password")
	if username == "" || password == "" {
		o.renderLogin(ctx, req, "用户名和密码不能为空")
		return
	}

	clientSDK := k8s.NewK8sClient()
	if _, err := clientSDK.Login2(username, password, true); err != nil {
		o.renderLogin(ctx, req, "用户名或密码错误")
		return
	}

	session := server.CreateSession(username)
	server.SetSessionCookie(ctx.Writer, ctx.Request, session)
	o.finishAuthorize(ctx, server, client, req, username)
}

func (o Oidc) Token(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}

	if err := ctx.Request.ParseForm(); err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_request", "invalid form")
		return
	}
	grantType := ctx.PostForm("grant_type")

	client, err := server.AuthenticateClient(ctx.Request)
	if err != nil {
		ctx.Header("WWW-Authenticate", `Basic realm="oidc"`)
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		return
	}

	switch grantType {
	case "authorization_code":
		o.handleAuthorizationCodeGrant(ctx, server, client)
	case "refresh_token":
		o.handleRefreshTokenGrant(ctx, server, client)
	default:
		o.oauthError(ctx, http.StatusBadRequest, "unsupported_grant_type", "only authorization_code and refresh_token are supported")
	}
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

func (o Oidc) handleAuthorizationCodeGrant(ctx *gin.Context, server *oidcservice.Server, client oidcservice.Client) {
	codeValue := ctx.PostForm("code")
	redirectURI := ctx.PostForm("redirect_uri")
	if codeValue == "" || redirectURI == "" {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_request", "code and redirect_uri are required")
		return
	}
	code, ok := server.ConsumeAuthorizationCode(codeValue)
	if !ok {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_grant", "invalid code")
		return
	}
	if code.ClientID != client.ClientID || code.RedirectURI != redirectURI {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_grant", "authorization code mismatch")
		return
	}
	if client.RequirePKCE || code.CodeChallenge != "" {
		verifier := ctx.PostForm("code_verifier")
		if verifier == "" || !server.VerifyPKCE(verifier, code.CodeChallenge) {
			o.oauthError(ctx, http.StatusBadRequest, "invalid_grant", "pkce verification failed")
			return
		}
	}

	result, err := server.CreateTokenPair(server.Issuer(ctx.Request), client, code.Username, code.Scopes, code.Nonce)
	if err != nil {
		o.oauthError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (o Oidc) handleRefreshTokenGrant(ctx *gin.Context, server *oidcservice.Server, client oidcservice.Client) {
	refreshToken := ctx.PostForm("refresh_token")
	if refreshToken == "" {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	claims, err := server.ParseRefreshToken(refreshToken, server.Issuer(ctx.Request))
	if err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}
	if claims.ClientID != client.ClientID {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_grant", "refresh token client mismatch")
		return
	}
	result, err := server.NewTokenPairFromRefreshToken(server.Issuer(ctx.Request), client, claims)
	if err != nil {
		o.oauthError(ctx, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (o Oidc) UserInfo(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		o.oauthError(ctx, http.StatusNotFound, "server_error", "oidc disabled")
		return
	}
	auth := ctx.GetHeader("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", "missing bearer token")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	claims, err := server.ParseAccessToken(token, server.Issuer(ctx.Request))
	if err != nil {
		o.oauthError(ctx, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	ctx.JSON(http.StatusOK, server.BuildUserClaims(claims.Username))
}

type authorizeRequest struct {
	ResponseType       string
	ClientID           string
	RedirectURI        string
	Scope              string
	State              string
	Nonce              string
	CodeChallenge      string
	CodeChallengeMode  string
	Prompt             string
	RequestedScopesRaw []string
}

func (o Oidc) parseAuthorizeRequest(ctx *gin.Context, server *oidcservice.Server) (authorizeRequest, oidcservice.Client, bool) {
	req := authorizeRequest{
		ResponseType:      ctx.Query("response_type"),
		ClientID:          ctx.Query("client_id"),
		RedirectURI:       ctx.Query("redirect_uri"),
		Scope:             ctx.Query("scope"),
		State:             ctx.Query("state"),
		Nonce:             ctx.Query("nonce"),
		CodeChallenge:     ctx.Query("code_challenge"),
		CodeChallengeMode: ctx.DefaultQuery("code_challenge_method", "S256"),
		Prompt:            ctx.Query("prompt"),
	}
	if req.ResponseType == "" && ctx.Request.Method == http.MethodPost {
		req.ResponseType = ctx.PostForm("response_type")
		req.ClientID = ctx.PostForm("client_id")
		req.RedirectURI = ctx.PostForm("redirect_uri")
		req.Scope = ctx.PostForm("scope")
		req.State = ctx.PostForm("state")
		req.Nonce = ctx.PostForm("nonce")
		req.CodeChallenge = ctx.PostForm("code_challenge")
		req.CodeChallengeMode = defaultString(ctx.PostForm("code_challenge_method"), "S256")
		req.Prompt = ctx.PostForm("prompt")
	}
	client, ok := server.FindClient(req.ClientID)
	if !ok {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_client", "unknown client")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	if req.RedirectURI == "" || !server.ValidateRedirectURI(client, req.RedirectURI) {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	if req.ResponseType != "code" {
		o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "unsupported_response_type", "")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	rawScopes := strings.Fields(req.Scope)
	if !contains(rawScopes, "openid") {
		o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "invalid_scope", "openid scope is required")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	req.RequestedScopesRaw = server.FilterScopes(client, rawScopes)
	if len(req.RequestedScopesRaw) == 0 {
		o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "invalid_scope", "no valid scopes requested")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	if req.CodeChallenge != "" && req.CodeChallengeMode != "S256" {
		o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "invalid_request", "only S256 is supported")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	if client.RequirePKCE && req.CodeChallenge == "" {
		o.redirectAuthorizeError(ctx, req.RedirectURI, req.State, "invalid_request", "code_challenge is required")
		return authorizeRequest{}, oidcservice.Client{}, false
	}
	return req, client, true
}

func (o Oidc) finishAuthorize(ctx *gin.Context, server *oidcservice.Server, client oidcservice.Client, req authorizeRequest, username string) {
	code := server.CreateAuthorizationCode(oidcservice.AuthorizationCode{
		ClientID:          client.ClientID,
		RedirectURI:       req.RedirectURI,
		Username:          username,
		Scopes:            req.RequestedScopesRaw,
		Nonce:             req.Nonce,
		CodeChallenge:     req.CodeChallenge,
		CodeChallengeMode: req.CodeChallengeMode,
	})
	redirectURL, err := url.Parse(req.RedirectURI)
	if err != nil {
		o.oauthError(ctx, http.StatusBadRequest, "invalid_request", "invalid redirect_uri")
		return
	}
	values := redirectURL.Query()
	values.Set("code", code.Code)
	if req.State != "" {
		values.Set("state", req.State)
	}
	redirectURL.RawQuery = values.Encode()
	ctx.Redirect(http.StatusFound, redirectURL.String())
}

func (o Oidc) redirectAuthorizeError(ctx *gin.Context, redirectURI string, state string, code string, description string) {
	redirectURL, err := url.Parse(redirectURI)
	if err != nil {
		o.oauthError(ctx, http.StatusBadRequest, code, description)
		return
	}
	values := redirectURL.Query()
	values.Set("error", code)
	if description != "" {
		values.Set("error_description", description)
	}
	if state != "" {
		values.Set("state", state)
	}
	redirectURL.RawQuery = values.Encode()
	ctx.Redirect(http.StatusFound, redirectURL.String())
}

func (o Oidc) oauthError(ctx *gin.Context, status int, code string, description string) {
	ctx.JSON(status, gin.H{
		"error":             code,
		"error_description": description,
	})
}

func (o Oidc) renderLogin(ctx *gin.Context, req authorizeRequest, errMsg string) {
	values := map[string]string{
		"response_type":       req.ResponseType,
		"client_id":           req.ClientID,
		"redirect_uri":        req.RedirectURI,
		"scope":               req.Scope,
		"state":               req.State,
		"nonce":               req.Nonce,
		"code_challenge":      req.CodeChallenge,
		"code_challenge_mode": req.CodeChallengeMode,
		"prompt":              req.Prompt,
	}
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
    <div class="meta">client_id={{.client_id}}</div>
    {{if .error}}<div class="error">{{.error}}</div>{{end}}
    <form method="post" action="/panel-api/v1/oidc/authorize/login">
      <input type="hidden" name="response_type" value="{{.response_type}}">
      <input type="hidden" name="client_id" value="{{.client_id}}">
      <input type="hidden" name="redirect_uri" value="{{.redirect_uri}}">
      <input type="hidden" name="scope" value="{{.scope}}">
      <input type="hidden" name="state" value="{{.state}}">
      <input type="hidden" name="nonce" value="{{.nonce}}">
      <input type="hidden" name="code_challenge" value="{{.code_challenge}}">
      <input type="hidden" name="code_challenge_method" value="{{.code_challenge_mode}}">
      <input type="hidden" name="prompt" value="{{.prompt}}">
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
		"response_type":       values["response_type"],
		"client_id":           values["client_id"],
		"redirect_uri":        values["redirect_uri"],
		"scope":               values["scope"],
		"state":               values["state"],
		"nonce":               values["nonce"],
		"code_challenge":      values["code_challenge"],
		"code_challenge_mode": values["code_challenge_mode"],
		"prompt":              values["prompt"],
		"error":               errMsg,
	}); err != nil {
		fmt.Fprint(ctx.Writer, template.HTMLEscapeString(err.Error()))
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
