package k3sregistry

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/app/k3s-registry/http/controller"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct{}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {
	p.RegisterHttpRoutes(httpServer) //

}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		// Registry API - 镜像仓库
		if facade.GetConfig().GetBool("registry.enabled") { //子用户 和 代理agent才开启
			registryGroup := engine.Group("")
			registryGroup.Use()
			{
				registryGroup.Any("/v2/*path", controller.Registry{}.Handler)
			}

			reg := engine.Group("/panel-api/v1/registry")
			reg.Use(middleware.Auth{}.Process)
			reg.Use()
			{
				reg.POST("/containers/:id/commit", controller.Commit{}.Run)
			}
			//TODO 子用户

		}
		// Registry API - 镜像仓库 因为要转发middleware.Proxy{}.Process 不能关闭路由registry.enabled
		patch := engine.Group("/panel-api/v1/registry/patch")
		patch.Use(middleware.Auth{}.Process, middleware.Proxy{}.Process)
		{
			patch.GET("/images/list", controller.Images{}.List)
			patch.PUT("/images/tag", controller.Images{}.Tag)
			patch.POST("/images/delete", controller.Images{}.Remove)
			patch.POST("/images/label", controller.Images{}.Label)
			patch.POST("/images/import", controller.Images{}.Import)
		}

		reg := engine.Group("/panel-api/v1/registry")
		reg.Use(middleware.Auth{}.Process)
		// reg.Use()
		{
			reg.GET("/server-info", controller.Registry{}.ServerInfo)
		}
	})
}
