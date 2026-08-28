package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/credential"
	"github.com/w7panel/w7panel/common/service/panelauth"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

// K8sAuth is deliberately separate from PanelAuth: a Kubernetes credential is
// accepted only by /k8s-proxy and never grants access to /panel-api.
type K8sAuth struct{ middleware.Abstract }

func (K8sAuth) Process(ctx *gin.Context) {
	k8sHeader := strings.TrimSpace(ctx.GetHeader("X-W7Panel-K8s-Token"))
	token := k8sHeader
	auth := ctx.GetHeader("Authorization")
	if token == "" && ckmAuthMode() == PanelAuthMode && strings.HasPrefix(auth, "Bearer ") {
		panelToken := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if principal, err := panelauth.Parse(panelToken); err == nil {
			k8sToken, _, issueErr := credential.IssueForPrincipal(ctx.Request.Context(), principal.Username, principal.PermissionName, 10*time.Minute)
			if issueErr == nil {
				ctx.Set("username", principal.Username)
				ctx.Set("k8s_token", k8sToken)
				ctx.Next()
				return
			}
		}
	}
	if token == "" && strings.HasPrefix(auth, "Bearer ") {
		token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "Kubernetes token 无效"})
		return
	}
	if token == "" || k8s.NewK8sClient().TokenReview(token) != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "Kubernetes token 无效"})
		return
	}
	ctx.Set("k8s_token", token)
	ctx.Next()
}
