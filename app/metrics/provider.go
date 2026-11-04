package metrics

import (

	// "sync"

	"github.com/gin-gonic/gin"
	console2 "github.com/w7panel/w7panel/app/metrics/console"
	controller2 "github.com/w7panel/w7panel/app/metrics/http/controller"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/audit"
	"github.com/w7panel/w7panel/common/service/k8s/metrics"
	"github.com/w7panel/w7panel/common/service/pluginproxy"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
	_ "modernc.org/sqlite"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {
	console.RegisterCommand(new(console2.MetricsCgroup))
	p.RegisterHttpRoutes(httpServer)
	p.startMetricsTicker()

}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		engine.GET("/panel-api/v1/metrics/usage/normal", middleware.Auth{}.Process, controller2.Metrics{}.Usage)
		engine.GET("/panel-api/v1/metrics/usage/disk", middleware.Auth{}.Process, controller2.Metrics{}.UsageDisk)
		engine.GET("/panel-api/v1/metrics/installed", middleware.Auth{}.Process, controller2.Metrics{}.VmOperatorInstalled)
		engine.GET("/panel-api/v1/metrics/state", middleware.Auth{}.Process, controller2.Metrics{}.MetricsState)
		engine.GET("/panel-api/v1/metrics/query-range", middleware.Auth{}.Process, controller2.Metrics{}.QueryRange)
		engine.GET("/panel-api/v1/metrics/plugin-identity", middleware.Auth{}.Process, func(ctx *gin.Context) {
			user := audit.CurrentUser(ctx)
			ctx.JSON(200, gin.H{"data": gin.H{"username": user.Username, "tenant": user.Tenant, "userMode": user.UserMode, "isAdmin": user.IsAdmin, "logsEnabled": audit.Enabled()}})
		})

		traffic := engine.Group("/panel-api/v1/traffic")
		traffic.Use(middleware.Auth{}.Process)
		proxy := pluginproxy.Handler("METRICS_TRAFFIC_URL", "http://metrics-traffic.default.svc:8080", "/panel-api/v1/traffic", "/api/v1/traffic")
		traffic.GET("/health", proxy)
		traffic.GET("/summary", proxy)
		traffic.GET("/series", proxy)
		traffic.GET("/pods", proxy)
		traffic.GET("/apps", proxy)
		traffic.GET("/domains", proxy)
		traffic.GET("/urls", proxy)
	})
}

func (p Provider) startMetricsTicker() {
	// go metrics.Start()
	go metrics.StartNodeMetrics()
	go metrics.StartPodMetrics()
	go metrics.StartCroupMetrics() // 启动cgroup监控
}
