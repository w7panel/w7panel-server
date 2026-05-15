package controller

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	oidcservice "github.com/w7panel/w7panel/common/service/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const oidcLoginPage = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>OIDC Login</title>
  <style>
    body {
      margin: 0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: linear-gradient(135deg, #f3f7fb 0%, #e8eef6 100%);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      color: #1f2937;
    }
    .card {
      width: min(420px, calc(100vw - 32px));
      background: #fff;
      border-radius: 16px;
      box-shadow: 0 20px 45px rgba(15, 23, 42, 0.12);
      padding: 28px;
      box-sizing: border-box;
    }
    h1 {
      margin: 0 0 8px;
      font-size: 24px;
    }
    p {
      margin: 0 0 20px;
      color: #4b5563;
      line-height: 1.5;
    }
    label {
      display: block;
      margin: 14px 0 6px;
      font-weight: 600;
      font-size: 14px;
    }
    input {
      width: 100%;
      box-sizing: border-box;
      border: 1px solid #d1d5db;
      border-radius: 10px;
      padding: 12px 14px;
      font-size: 14px;
    }
    button {
      width: 100%;
      margin-top: 18px;
      border: 0;
      border-radius: 10px;
      background: #111827;
      color: #fff;
      font-size: 15px;
      font-weight: 600;
      padding: 12px 14px;
      cursor: pointer;
    }
    .error {
      margin-bottom: 14px;
      padding: 10px 12px;
      border-radius: 10px;
      background: #fef2f2;
      color: #b91c1c;
      font-size: 14px;
    }
    .meta {
      margin-top: 16px;
      font-size: 12px;
      color: #6b7280;
      word-break: break-all;
    }
  </style>
</head>
<body>
  <div class="card">
    <h1>登录授权</h1>
    <p>继续为客户端 <strong>{{.ClientID}}</strong> 完成登录授权。</p>
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	    <form method="post" action="/panel-api/v1/oidc/authorize/login?authRequestID={{.AuthRequestID}}">
      <label for="username">用户名</label>
      <input id="username" name="username" type="text" autocomplete="username" value="{{.Username}}" required>
      <label for="password">密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password" required>
      <button type="submit">登录并授权</button>
    </form>
    <div class="meta">authRequestID: {{.AuthRequestID}}</div>
  </div>
</body>
</html>`

type oidcLoginPageData struct {
	AuthRequestID string
	ClientID      string
	Username      string
	Error         string
}

func (o Oidc) LoginPage(ctx *gin.Context) {
	server, authReqID, authReq, ok := o.loadAuthRequest(ctx)
	if !ok {
		return
	}
	if username, ok := oidcUsernameFromToken(ctx); ok {
		if err := server.CompleteAuthRequest(authReqID, username); err == nil {
			ctx.Redirect(http.StatusFound, server.CallbackURL(ctx.Request.Context(), authReqID))
			return
		}
	}
	o.renderLoginPage(ctx, oidcLoginPageData{
		AuthRequestID: authReqID,
		ClientID:      authReq.GetClientID(),
	})
}

func (o Oidc) Login(ctx *gin.Context) {
	server, authReqID, authReq, ok := o.loadAuthRequest(ctx)
	if !ok {
		return
	}

	type loginForm struct {
		Username string `form:"username" binding:"required"`
		Password string `form:"password" binding:"required"`
	}
	var form loginForm
	if err := ctx.ShouldBind(&form); err != nil {
		o.renderLoginPage(ctx, oidcLoginPageData{
			AuthRequestID: authReqID,
			ClientID:      authReq.GetClientID(),
			Username:      form.Username,
			Error:         "用户名和密码不能为空",
		})
		return
	}

	if err := server.Login(ctx.Request.Context(), authReqID, form.Username, form.Password); err != nil {
		o.renderLoginPage(ctx, oidcLoginPageData{
			AuthRequestID: authReqID,
			ClientID:      authReq.GetClientID(),
			Username:      form.Username,
			Error:         err.Error(),
		})
		return
	}
	ctx.Redirect(http.StatusFound, server.CallbackURL(ctx.Request.Context(), authReqID))
}

func (o Oidc) loadAuthRequest(ctx *gin.Context) (*oidcservice.Server, string, op.AuthRequest, bool) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return nil, "", nil, false
	}
	authReqID := ctx.Query("authRequestID")
	if authReqID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing authRequestID"})
		return nil, "", nil, false
	}
	authReq, err := server.AuthRequestByID(ctx.Request.Context(), authReqID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid authRequestID", "error_description": err.Error()})
		return nil, "", nil, false
	}
	return server, authReqID, authReq, true
}

func (o Oidc) renderLoginPage(ctx *gin.Context, data oidcLoginPageData) {
	tpl, err := template.New("oidc-login").Parse(oidcLoginPage)
	if err != nil {
		ctx.String(http.StatusInternalServerError, "template render failed")
		return
	}
	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "text/html; charset=utf-8")
	if err := tpl.Execute(ctx.Writer, data); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		ctx.Error(err)
	}
}

func oidcUsernameFromToken(ctx *gin.Context) (string, bool) {
	token := helper.GetToken(ctx)
	if token == "" {
		return "", false
	}
	username, _ := k8s.NewK8sToken(token).GetSaName()
	if username == "" {
		return "", false
	}
	return username, true
}
