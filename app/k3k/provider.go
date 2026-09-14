package k3k

import (
	"github.com/gin-gonic/gin"
	controller2 "github.com/w7panel/w7panel/app/k3k/http/controller"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {

	p.RegisterHttpRoutes(httpServer)

	if helper.IsChildAgent() {
		go k3k.SyncMicroApp()
	}
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		k3kGroup := engine.Group("/panel-api/v1/k3k")
		{
			k3kGroup.POST("/sync-ingress", controller2.K3k{}.SyncIngress)
			k3kGroup.POST("/sync-configmap", controller2.K3k{}.SyncConfigmap)
			k3kGroup.POST("/sync-mcpbridge", controller2.K3k{}.SyncMcpBridge)
			k3kGroup.POST("/sync-secret", controller2.K3k{}.SyncSecret)
			k3kGroup.POST("/sync-down-static", controller2.K3k{}.SyncDownStatic)
			k3kGroup.POST("/sync-microapp", controller2.K3k{}.SyncMicroApp)
			k3kGroup.POST("/sync-site", controller2.K3k{}.SyncSite)
		}
	})
}
