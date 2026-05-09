package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	oidcservice "github.com/w7panel/w7panel/common/service/oidc"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Oidc struct {
	controller.Abstract
}

func (o Oidc) Handle(ctx *gin.Context) {
	server, err := oidcservice.GetServer()
	if err != nil || server == nil || !server.Enabled() {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
		return
	}
	server.ServeHTTP(ctx.Writer, ctx.Request)
}
