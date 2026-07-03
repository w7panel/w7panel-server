package auth

import (
	"github.com/gin-gonic/gin"
	app "github.com/w7panel/w7panel/app/auth/console"
	controller2 "github.com/w7panel/w7panel/app/auth/http/controller"
	k3kController "github.com/w7panel/w7panel/app/k3k/http/controller"
	"github.com/w7panel/w7panel/common/middleware"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {
	console.RegisterCommand(new(app.Register))
	console.RegisterCommand(new(app.Cluster))
	console.RegisterCommand(new(app.Site))
	console.RegisterCommand(new(app.SiteZpkHttp))
	console.RegisterCommand(new(app.SiteZpk))
	console.RegisterCommand(new(app.CreateInnerDb))
	console.RegisterCommand(new(app.Unzip))
	console.RegisterCommand(new(app.OidcClientCreate))

	p.RegisterHttpRoutes(httpServer)
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		oidcGroup := engine.Group("/panel-api/v1/oidc")
		{
			oidcGroup.Any("/.well-known/openid-configuration", controller2.Oidc{}.Discovery)
			oidcGroup.Any("/jwks", controller2.Oidc{}.Handle)
			// oidcGroup.POST("/register", controller2.Oidc{}.RegisterClient)
			// oidcGroup.GET("/register/:clientId", controller2.Oidc{}.GetClient)
			// oidcGroup.PUT("/register/:clientId", controller2.Oidc{}.UpdateClient)
			// oidcGroup.DELETE("/register/:clientId", controller2.Oidc{}.DeleteClient)

			oidcGroup.Any("/authorize", controller2.Oidc{}.Handle)
			oidcGroup.Any("/token", controller2.Oidc{}.Handle)
			oidcGroup.Any("/userinfo", controller2.Oidc{}.Handle)
			//统一路由
			oidcGroup.POST("/js-code", middleware.Auth{}.Process, controller2.Oidc{}.AuthorizeCode)
			oidcGroup.POST("/redirect-uri", middleware.Auth{}.Process, controller2.Oidc{}.GetRedirectURI)

		}

		engine.POST("/panel-api/v1/login", controller2.Auth{}.Login)

		localApiGroup := engine.Group("/panel-api/v1/auth").Use(middleware.Cors{}.Process)
		{
			localApiGroup.POST("/login", middleware.ConsoleSignature{}.Process, controller2.Auth{}.LoginBySign)
			localApiGroup.POST("/api-token", controller2.APIToken{}.Exchange)
			localApiGroup.POST("/register", controller2.Auth{}.Register)
			localApiGroup.POST("/refresh-token2", controller2.Auth{}.RefreshToken2)
			localApiGroup.POST("/init-user", controller2.Auth{}.InitUser)
			localApiGroup.POST("/reset-password", middleware.Auth{}.Process, controller2.Auth{}.ResetPassword)
			localApiGroup.POST("/reset-password-current", middleware.Auth{}.Process, controller2.Auth{}.ResetPasswordCurrent) //设置当前登录用户密码

			localApiGroup.GET("/console/oauth", controller2.Console{}.Redirect)

			localApiGroup.GET("/console/login", controller2.Auth{}.ConsoleLogin)
			localApiGroup.GET("/console/bind", middleware.Auth{}.Process, controller2.Console{}.BindConsole)
			localApiGroup.GET("/console/info", middleware.Auth{}.Process, controller2.Console{}.Info)

			localApiGroup.GET("/userinfo", middleware.Auth{}.Process, k3kController.K3k{}.Info)
			// 不需要创始人权限
			// localApiGroup.GET("/console/code/:code", middleware.Auth{}.Process, controller2.Console{}.ProxyCouponCode)
			// localApiGroup.Any("/console/proxy/*path", middleware.Auth{}.Process, controller2.Console{}.Proxy)

			localApiGroup.Any("/console/proxy/*path", middleware.Auth{}.Process, controller2.Console{}.Proxy)
			localApiGroup.GET("/permissions/routes", middleware.Auth{}.Process, func(ctx *gin.Context) {
				ctx.JSON(200, gin.H{
					"code": 200,
					"data": permissionservice.RoutesFromGin(engine.Routes()),
				})
			})

			localApiGroup.POST("/console/register-to-console", middleware.Auth{}.Process, controller2.Console{}.RegisterToConsole) //不能proxy 需要root kubeconfig
			//不能proxy 需要root kubeconfig
			localApiGroup.POST("/console/import-cert", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.ImportCert)
			localApiGroup.POST("/console/verify-cert", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.VerifyCert)
			localApiGroup.POST("/console/import-cert-console", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.ImportCertConsole)
			// 即将废弃 使用site crd
			localApiGroup.POST("/console/register-zpk-site" /* middleware.ConsoleSignature{}.Process */, controller2.Site{}.RegisterZpkSite)
		}

		engine.POST("/panel-api/v1/code", middleware.Auth{}.Process, controller2.Oidc{}.AuthorizeCode)
		engine.POST("/panel-api/v1/callback-url", middleware.Auth{}.Process, controller2.Oidc{}.GetRedirectURI)
		engine.GET("/panel-api/v1/js-cloud-code", middleware.Auth{}.Process, controller2.Console{}.JsCloudCode)

	})
}
