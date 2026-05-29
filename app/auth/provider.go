package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	app "github.com/w7panel/w7panel/app/auth/console"
	controller2 "github.com/w7panel/w7panel/app/auth/http/controller"
	k3kController "github.com/w7panel/w7panel/app/k3k/http/controller"
	"github.com/w7panel/w7panel/common/middleware"
	console2 "github.com/w7panel/w7panel/common/service/console"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
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
	if facade.GetConfig().GetBool("site.enabled") {
		// slog.Info("site refresh token timer")
		// go p.RefreshCDToken() // 用户登录时候 触发刷新token
	}
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	server.RegisterRouters(func(engine *gin.Engine) {
		oidcGroup := engine.Group("/panel-api/v1/oidc")
		{
			oidcGroup.Any("/.well-known/openid-configuration", controller2.Oidc{}.Discovery)
			oidcGroup.Any("/jwks", controller2.Oidc{}.Handle)
			oidcGroup.POST("/register", controller2.Oidc{}.RegisterClient)
			oidcGroup.GET("/register/:clientId", controller2.Oidc{}.GetClient)
			oidcGroup.PUT("/register/:clientId", controller2.Oidc{}.UpdateClient)
			oidcGroup.DELETE("/register/:clientId", controller2.Oidc{}.DeleteClient)
			oidcGroup.GET("/authorize/login", controller2.Oidc{}.LoginPage)
			oidcGroup.POST("/authorize/login", controller2.Oidc{}.Login)
			oidcGroup.Any("/authorize", controller2.Oidc{}.Handle)
			// oidcGroup.Any("/authorize/*path", controller2.Oidc{}.Handle)
			oidcGroup.Any("/token", controller2.Oidc{}.Handle)
			oidcGroup.Any("/userinfo", controller2.Oidc{}.Handle)
			//统一路由
			//http://127.0.0.1:9007/authorize?client_id=default&redirect_uri=http://127.0.0.1:3000/callback111&scope=openid&response_type=code
			oidcGroup.POST("/js-code", middleware.Auth{}.Process, controller2.Oidc{}.AuthorizeCode)
			oidcGroup.POST("/redirect-uri", middleware.Auth{}.Process, controller2.Oidc{}.GetRedirectURI)

		}

		engine.POST("/panel-api/v1/login", controller2.Auth{}.Login)

		localApiGroup := engine.Group("/panel-api/v1/auth").Use(middleware.Cors{}.Process)
		{
			localApiGroup.POST("/login", middleware.ConsoleSignature{}.Process, controller2.Auth{}.LoginBySign)
<<<<<<< HEAD
			// localApiGroup.POST("/register", controller2.Auth{}.Register) //去掉注册功能 走控制台注册
=======
			localApiGroup.POST("/register", controller2.Auth{}.Register)
>>>>>>> dev-v1
			// localApiGroup.POST("/console/k3k-register", middleware.Auth{}.Process, controller2.Auth{}.RegisterUseUid)
			// localApiGroup.POST("/refresh-token", middleware.Auth{}.Process, controller2.Auth{}.RefreshToken) //废弃
			localApiGroup.POST("/refresh-token2", controller2.Auth{}.RefreshToken2)
			localApiGroup.POST("/init-user", controller2.Auth{}.InitUser)
			localApiGroup.POST("/reset-password", middleware.Auth{}.Process, controller2.Auth{}.ResetPassword)
			localApiGroup.POST("/reset-password-current", middleware.Auth{}.Process, controller2.Auth{}.ResetPasswordCurrent) //设置当前登录用户密码

			localApiGroup.GET("/console/oauth", controller2.Console{}.Redirect)

			localApiGroup.GET("/console/login", controller2.Auth{}.ConsoleLogin)
			localApiGroup.GET("/console/bind", middleware.Auth{}.Process /*middleware.BindConsole{}.Process, middleware.Proxy{}.Process, */, controller2.Console{}.BindConsole)
			localApiGroup.GET("/console/info", middleware.Auth{}.Process, controller2.Console{}.Info)
			localApiGroup.GET("/userinfo", middleware.Auth{}.Process, k3kController.K3k{}.Info)
			// 不需要创始人权限
			localApiGroup.GET("/console/code/:code", middleware.Auth{}.Process, controller2.Console{}.ProxyCouponCode)
			localApiGroup.Any("/console/proxy/*path", middleware.NewAuth("founder").Process, controller2.Console{}.Proxy)

			localApiGroup.POST("/console/register-to-console", middleware.Auth{}.Process, controller2.Console{}.RegisterToConsole) //不能proxy 需要root kubeconfig
			//不能proxy 需要root kubeconfig
			localApiGroup.POST("/console/thirdparty-cd-token", middleware.Auth{}.Process, controller2.Console{}.ThirdPartyCDToken)
			localApiGroup.POST("/console/import-cert", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.ImportCert)
			localApiGroup.POST("/console/verify-cert", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.VerifyCert)
			localApiGroup.POST("/console/import-cert-console", middleware.Auth{}.Process /*middleware.Proxy{}.Process, */, controller2.Console{}.ImportCertConsole)
			localApiGroup.POST("/console/register-zpk-site" /* middleware.ConsoleSignature{}.Process */, controller2.Site{}.RegisterZpkSite)
<<<<<<< HEAD
			registerHawkTestRoute(localApiGroup, middleware.Hawk{}.Process)
=======
>>>>>>> dev-v1
		}

		//直接获取code 用于OIDC //旧路由
		// engine.GET("/.well-known/openid-configuration", controller2.Oidc{}.Handle) //框架限制
		engine.POST("/panel-api/v1/code", middleware.Auth{}.Process, controller2.Oidc{}.AuthorizeCode)
		//http://127.0.0.1:9007/authorize?client_id=default&redirect_uri=http://127.0.0.1:3000/callback111&scope=openid&response_type=code
		engine.POST("/panel-api/v1/callback-url", middleware.Auth{}.Process, controller2.Oidc{}.GetRedirectURI)

<<<<<<< HEAD
	})
}

func registerHawkTestRoute(group gin.IRoutes, hawkMiddleware gin.HandlerFunc) {
	group.GET("/hawk-test", hawkMiddleware, func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{
			"ok":            true,
			"apiClientId":   ctx.GetString("api_client_id"),
			"apiClientName": ctx.GetString("api_client_name"),
			"hawkClientId":  ctx.GetString("hawk_client_id"),
		})
=======
>>>>>>> dev-v1
	})
}

func (p Provider) RefreshCDToken() {
	// 一个1分钟的定时器 定时执行console.RefreshToken方法
	tokenResolution := facade.Config.GetDuration("site.token_refresh_resolution")
	time := time.NewTicker(tokenResolution)

	// go console2.RefreshCDToken()

	go func() {
		for range time.C {
			// 刷新token
			slog.Info("刷新token")

			err := console2.VerifyDefaultLicense(true)
			if err != nil {
				slog.Error("刷新license失败", "err", err)
			}
			// err = console2.ReVerifyLicense(sdk)
			// if err != nil {
			// 	slog.Error("刷新license失败", "err", err)
			// }
		}
	}()

}
