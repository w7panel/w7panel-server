package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s/credential"
	"github.com/w7panel/w7panel/common/service/panelauth"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type K8sCredential struct{ controller.Abstract }

// Token returns a short-lived credential for the already authenticated panel
// principal. The caller cannot select another user, role, or cluster.
func (K8sCredential) Token(ctx *gin.Context) {
	principal, ok := ctx.Get("panel_principal")
	if !ok {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "请登录"})
		return
	}
	p := principal.(panelauth.Principal)
	sourceToken := strings.TrimSpace(ctx.GetHeader("X-W7Panel-K8s-Token"))
	token, expiresAt, err := credential.IssueForPrincipalFromToken(ctx.Request.Context(), p.Username, p.PermissionName, sourceToken, 10*time.Minute)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "msg": "生成 Kubernetes 凭据失败"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": gin.H{"token": token, "expiresAt": expiresAt}})
}
