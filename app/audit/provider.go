package audit

import (
	"github.com/gin-gonic/gin"
	controller "github.com/w7panel/w7panel/app/audit/http/controller"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {
	p.RegisterHttpRoutes(httpServer)
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		group := engine.Group("/panel-api/v1/audit")
		group.Use(middleware.Auth{}.Process)
		{
			group.GET("/logs/status", controller.Audit{}.Status)
			group.GET("/login-logs", controller.Audit{}.LoginLogs)
			group.GET("/operation-logs", controller.Audit{}.OperationLogs)
		}
	})
}
