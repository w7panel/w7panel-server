package k3k

import (
	"github.com/gin-gonic/gin"
	consoleShell "github.com/w7panel/w7panel/app/k3k/console"
	controller2 "github.com/w7panel/w7panel/app/k3k/http/controller"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {

	console.RegisterCommand(new(consoleShell.QxUpgrade))

	p.RegisterHttpRoutes(httpServer)

	if helper.IsChildAgent() {
		go k3k.SyncMicroApp()
	}
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		k3kGroup := engine.Group("/panel-api/v1/k3k") //.Use(middleware.Cors{}.Process)
		{
			k3kGroup.GET("/info", middleware.Auth{}.Process, controller2.K3k{}.Info)           // 登录信息
			k3kGroup.POST("/init", middleware.Auth{}.Process, controller2.K3k{}.ReInitCluster) // 初始化集群
			// k3kGroup.POST("/whjob", middleware.Auth{}.Process, controller2.K3k{}.WhJob)        // 重新新建救援任务
			// k3kGroup.POST("/init-cluster", middleware.Auth{}.Process, controller2.K3k{}.ReInitClusterSuper) // 创始人初始化集群
			k3kGroup.POST("/sync-ingress", controller2.K3k{}.SyncIngress)        //
			k3kGroup.POST("/sync-configmap", controller2.K3k{}.SyncConfigmap)    //
			k3kGroup.POST("/sync-mcpbridge", controller2.K3k{}.SyncMcpBridge)    //
			k3kGroup.POST("/sync-secret", controller2.K3k{}.SyncSecret)          //
			k3kGroup.POST("/sync-down-static", controller2.K3k{}.SyncDownStatic) //
			k3kGroup.POST("/sync-microapp", controller2.K3k{}.SyncMicroApp)      //microapp同步到子集群
			// 主集群login 需要验证是否founder
			k3kGroup.POST("/login", middleware.Auth{}.Process, controller2.K3k{}.Login) //

			// k3kGroup.POST("/wh", middleware.Auth{}.Process, controller2.K3k{}.WhMoshi) // 维护模式 切换

			k3kGroup.POST("/order/notify", controller2.Order{}.OrderNotify) // 支付回调 不需要登录

			k3kGroup.POST("/order/license", middleware.Auth{}.Process, controller2.Order{}.CreateLicenseOrder) // 面板授权购买

			k3kGroup.GET("/cvm", middleware.Auth{}.Process, controller2.Ckm{}.List)                          // cvm列表 旧版路由
			k3kGroup.GET("/cvm/v1/:namespace/info/:name", middleware.Auth{}.Process, controller2.Ckm{}.Info) // cvm详情 旧版路由

			k3kGroup.GET("/ckm", middleware.Auth{}.Process, controller2.Ckm{}.List)                          // cvm列表 新
			k3kGroup.GET("/ckm/v1/:namespace/info/:name", middleware.Auth{}.Process, controller2.Ckm{}.Info) // cvm详情 新

			k3kGroup.POST("/cvm/:namespace/action/:name/login", middleware.Auth{}.Process, controller2.K3k{}.LoginCvm) // cvm 登录

		}

		k3kGroup1 := engine.Group("/panel-api/v1/k3k/overselling") //.Use(middleware.Cors{}.Process)
		{
			k3kGroup1.GET("/config", middleware.Auth{}.Process, controller2.OverSelling{}.OverSellingConfig)         // 获取超卖配置
			k3kGroup1.GET("/current-resource", middleware.Auth{}.Process, controller2.OverSelling{}.CurrentResource) // 获取超卖百分比*当前集群资源
		}

		k8kGroup := engine.Group("/panel-api/v1") //.Use(middleware.Cors{}.Process)
		{
			k8kGroup.GET("/userinfo", middleware.Auth{}.Process, controller2.K3k{}.Info) // 登录信息
			k8kGroup.GET("/idc-list", controller2.Ckm{}.IdcResource)                     // IDC资源列表
		}

		k8kGroupOld := engine.Group("/k8s/k3k") //.Use(middleware.Cors{}.Process)
		{
			k8kGroupOld.POST("/order/notify", controller2.Order{}.OrderNotify) // 旧版通知
		}
	})
}
