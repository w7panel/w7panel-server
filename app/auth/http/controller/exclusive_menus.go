package controller

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	permission "github.com/w7panel/w7panel/common/service/k8s/permission"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type ExclusiveMenus struct{}

func (ExclusiveMenus) Get(ctx *gin.Context) {
	policy, err := permission.LoadExclusiveMenus(ctx.Request.Context(), k8s.NewK8sClient().Sdk)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "专属菜单加载失败"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": policy})
}

func (ExclusiveMenus) Put(ctx *gin.Context) {
	if helper.IsChildAgent() || ctx.GetString("user_mode") != permission.FounderPermissionName {
		ctx.AbortWithStatus(http.StatusForbidden)
		return
	}
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 1<<20)
	var policy permission.ExclusiveMenus
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&policy); err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "专属菜单数据无效"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "专属菜单数据无效"})
		return
	}
	if err := policy.Validate(); err != nil {
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	saved, err := permission.SaveExclusiveMenus(ctx.Request.Context(), k8s.NewK8sClient().Sdk, policy)
	if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
		ctx.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "规则已变更，请刷新后重试"})
		return
	}
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "专属菜单保存失败"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": saved})
}
