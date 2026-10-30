package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
)

func (self Auth) PermissionExclusives(ctx *gin.Context) {
	policy, err := permissionservice.LoadExclusivePolicy(ctx.Request.Context(), k8s.NewK8sClient().Sdk)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	self.JsonResponseWithoutError(ctx, gin.H{"rules": policy.Rules, "group": ctx.GetString("user_mode")})
}
