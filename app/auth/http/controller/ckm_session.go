package controller

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/ckmsession"
	"github.com/w7panel/w7panel/common/service/panelauth"
)

type CKMSession struct{}

func (CKMSession) Create(ctx *gin.Context) {
	var body struct {
		Namespace string `json:"namespace" binding:"required"`
		Name      string `json:"name" binding:"required"`
		K8sToken  string `json:"k8sToken" binding:"required"`
	}
	if ctx.ShouldBindJSON(&body) != nil {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "缺少 CKM 登录参数"})
		return
	}
	value, _ := ctx.Get("panel_principal")
	actor, ok := value.(panelauth.Principal)
	if !ok {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	token, expiresAt, err := ckmsession.Exchange(ctx.Request.Context(), actor, body.Namespace, body.Name, body.K8sToken)
	if err != nil {
		slog.Warn("CKM panel login denied", "actor", actor.Username, "namespace", body.Namespace, "ckm", body.Name)
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无权登录该 CKM，或登录凭据已失效"})
		return
	}
	slog.Info("CKM panel login", "actor", actor.Username, "namespace", body.Namespace, "ckm", body.Name)
	// Do not set the host's session cookie for an embedded CKM session.
	ctx.JSON(http.StatusOK, gin.H{"token": token, "expiresAt": expiresAt})
}
