package audit

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/pluginproxy"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server) {
	p.RegisterHttpRoutes(httpServer)
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		group := engine.Group("/panel-api/v1/audit")
		group.Use(middleware.Auth{}.Process)
		{
			proxy := pluginproxy.Handler("METRICS_AUDIT_URL", "http://metrics-audit.default.svc:8080", "/panel-api/v1/audit", "/api/v1/audit")
			group.GET("/logs/status", proxy)
			group.GET("/login-logs", proxy)
			group.GET("/operation-logs", proxy)
		}
	})
}
