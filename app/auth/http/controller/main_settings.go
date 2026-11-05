package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s/site"
	"github.com/w7panel/w7panel/common/service/panelauth"
)

type MainSettings struct{}

func (MainSettings) Get(ctx *gin.Context) {
	value, _ := ctx.Get("panel_principal")
	principal, ok := value.(panelauth.Principal)
	if !ok || !helper.IsChildAgent() || principal.TokenUse != panelauth.TokenUsePanel {
		ctx.AbortWithStatus(http.StatusForbidden)
		return
	}
	data, err := site.FetchMainSettings(ctx.Request.Context())
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, site.ErrMainSettingsUnavailable) {
			status = http.StatusServiceUnavailable
		}
		ctx.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": http.StatusOK, "data": gin.H{
		"siteSetting": data["siteSetting"], "configMaps": data["configMaps"], "domainParse": data["domainParse"],
	}})
}
