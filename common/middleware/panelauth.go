package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/credential"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	"github.com/w7panel/w7panel/common/service/panelauth"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

const panelSessionCookie = "w7panel_session"

type PanelAuth struct{ middleware.Abstract }

func (PanelAuth) Process(ctx *gin.Context) {
	raw := panelToken(ctx.Request)
	principal, err := panelauth.Parse(raw)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "请登录"})
		return
	}

	sdk := k8s.NewK8sClient().Sdk
	var permissionName = principal.PermissionName
	if principal.TokenUse == panelauth.TokenUseExternalAPI {
		permissionName = permissionservice.APIPermissionName
	}
	user, err := userservice.Get(ctx.Request.Context(), sdk, principal.Username)
	if err == nil && principal.TokenUse == panelauth.TokenUsePanel {
		permissionName = user.Spec.PermissionName
		if permissionName == "" {
			permissionName = principal.PermissionName
		}
	}
	permission, err := permissionservice.Get(ctx.Request.Context(), sdk, permissionservice.NormalizePermissionName(permissionName))
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "msg": "没有权限"})
		return
	}
	allowed, err := permissionservice.AuthorizePanelAPIWithPermission(ctx.Request.Context(), sdk, permission, ctx.Request.Method, ctx.Request.URL.Path)
	if err != nil || !allowed {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "msg": "没有权限"})
		return
	}
	role := principal.Role
	if user != nil && role == "" {
		role = user.Spec.Role
	}
	ctx.Set("panel_principal", *principal)
	ctx.Set("username", principal.Username)
	ctx.Set("permission_name", permission.Name)
	ctx.Set("user_mode", role)
	// Compatibility for legacy cluster controllers still registered below
	// /panel-api. The credential is minted server-side and never comes from the
	// client request; migrated routes use K8sAuth under /k8s-proxy directly.
	if requiresLegacyK8sCredential(ctx.Request.URL.Path) {
		k8sToken, _, err := credential.IssueForPrincipal(ctx.Request.Context(), principal.Username, permission.Name, 10*time.Minute)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "msg": "生成 Kubernetes 凭据失败"})
			return
		}
		ctx.Set("k8s_token", k8sToken)
	}
	ctx.Next()
}

func SetPanelSession(ctx *gin.Context, token string, maxAge int) {
	ctx.SetCookie(panelSessionCookie, token, maxAge, "/", "", false, true)
}

func panelToken(req *http.Request) string {
	if panelTokenHeaderEnabled() {
		if token := strings.TrimSpace(req.Header.Get("X-W7Panel-Token")); token != "" {
			return token
		}
	}
	if auth := req.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	cookie, err := req.Cookie(panelSessionCookie)
	if err == nil {
		return cookie.Value
	}
	return ""
}

func requiresLegacyK8sCredential(path string) bool {
	return !strings.HasPrefix(path, "/panel-api/v1/auth/") &&
		!strings.HasPrefix(path, "/panel-api/v1/oidc/") &&
		!strings.HasPrefix(path, "/panel-api/v1/noauth/")
}
