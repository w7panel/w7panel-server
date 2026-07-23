package controller

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/registry/containerd"

	bm "github.com/w7panel/w7panel/common/service/k8s/buildimage"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Registry struct {
	controller.Abstract
}

var regisry http.Handler

func init() {
	reg, err := containerd.InitReigstry(context.Background())
	if err != nil {
		// slog.Error("init registry err", "err", err)
		return
	}
	regisry = reg
}

// Catalog 返回镜像列表
func (self Registry) Handler(ctx *gin.Context) {
	if regisry != nil {
		regisry.ServeHTTP(ctx.Writer, ctx.Request)
		return
	}
	err := errors.New("not support")
	self.JsonResponseWithServerError(ctx, err)
}

func (self Registry) ServerInfo(http *gin.Context) {
	hostIp := http.Query("hostIp")
	token := http.MustGet("k8s_token").(string)
	info, err := bm.PanelRegistryServerInfo(token, hostIp)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, info)
}
