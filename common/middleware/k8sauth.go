package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

// K8sAuth is deliberately separate from PanelAuth: a Kubernetes credential is
// accepted only by /k8s-proxy and never grants access to /panel-api.
type K8sAuth struct{ middleware.Abstract }

func (K8sAuth) Process(ctx *gin.Context) {
	token := strings.TrimSpace(ctx.GetHeader("X-W7Panel-K8s-Token"))
	if token == "" {
		auth := ctx.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
	}
	if token == "" || k8s.NewK8sClient().TokenReview(token) != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "Kubernetes token 无效"})
		return
	}
	ctx.Set("k8s_token", token)
	ctx.Next()
}
