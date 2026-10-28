package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/oidc"
	"github.com/w7panel/w7panel/common/service/panelauth"
	"github.com/w7panel/w7panel/common/service/user"
)

type MCPAuth struct{}

func (MCPAuth) Process(ctx *gin.Context) {
	if os.Getenv("LOCAL_MOCK") == "true" || os.Getenv("LOCAL_MOCK") == "1" {
		Auth{}.Process(ctx)
		return
	}
	raw := panelToken(ctx.Request)
	if principal, err := panelauth.Parse(raw); err == nil {
		processPanelPrincipal(ctx, principal)
		return
	}
	server, err := oidc.GetServer()
	if err == nil && server != nil && server.Enabled() {
		if username, err := server.VerifyMCPAccessToken(ctx.Request, raw); err == nil {
			if _, err := user.Get(ctx.Request.Context(), k8s.NewK8sClient().Sdk, username); err != nil {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "msg": "用户不存在或无权限"})
				return
			}
			processPanelPrincipal(ctx, &panelauth.Principal{Username: username, TokenUse: panelauth.TokenUsePanel})
			return
		}
	}
	if helper.IsChildAgent() && ckmAuthMode() == K8sAuthMode {
		Auth{}.Process(ctx)
		return
	}
	if server != nil && server.Enabled() {
		resource := server.MCPResource(ctx.Request)
		metadata := strings.TrimSuffix(resource, "/panel-api/v1/mcp") + "/.well-known/oauth-protected-resource/panel-api/v1/mcp"
		ctx.Header("WWW-Authenticate", `Bearer resource_metadata="`+metadata+`"`)
	}
	ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "请登录"})
}
