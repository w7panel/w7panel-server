package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
)

func allowExclusiveAPI(ctx *gin.Context, sdk *k8s.Sdk, clusterType string) bool {
	policy, err := permissionservice.LoadExclusivePolicy(ctx.Request.Context(), sdk)
	if err != nil || !policy.AllowsAPI(clusterType, ctx.Request.Method, ctx.Request.URL.Path) {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "msg": "专属 API 无权限或策略无效"})
		return false
	}
	return true
}
