package main

import (
	"bytes"
	_ "embed"
	"io"
	"log"
	"log/slog"
	stdhttp "net/http"
	"os"

	"github.com/w7panel/w7panel/app/application"
	"github.com/w7panel/w7panel/app/application/http/controller"
	auditapp "github.com/w7panel/w7panel/app/audit"
	"github.com/w7panel/w7panel/app/auth"
	"github.com/w7panel/w7panel/app/k3k"
	k3sregistry "github.com/w7panel/w7panel/app/k3s-registry"
	metricsapp "github.com/w7panel/w7panel/app/metrics"
	"github.com/w7panel/w7panel/app/zpk"
	commonmiddleware "github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/k8s"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/grafana/pyroscope-go"
	"github.com/spf13/viper"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	app "github.com/we7coreteam/w7-rangine-go/v2/src"
	corehelper "github.com/we7coreteam/w7-rangine-go/v2/src/core/helper"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/response"
	cachecontrol "go.eigsys.de/gin-cachecontrol/v2"
	"go.uber.org/automaxprocs/maxprocs"
)

//go:embed config.yaml
var ConfigFileContent []byte

var Asset = (os.DirFS(os.Getenv("KO_DATA_PATH")))

func pyroscope2() {
	serverAddress := facade.Config.GetString("pyroscope.server_address")
	helmreleaseName := facade.Config.GetString("app.helm_release_name")
	_, err := pyroscope.Start(pyroscope.Config{
		ApplicationName: "w7panel-offline",
		ServerAddress:   serverAddress,
		Logger:          pyroscope.StandardLogger,
		Tags:            map[string]string{"releasename": helmreleaseName},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,
			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	})
	if err != nil {
		log.Fatalf("error starting pyroscope profiler: %v", err)
	}
}

func main() {
	maxprocs.Set(maxprocs.Logger(nil))

	newApp := app.NewApp(app.Option{
		DefaultConfigLoader: func(config *viper.Viper) {
			config.SetConfigType("yaml")
			err := config.MergeConfig(bytes.NewReader(corehelper.ParseConfigContentEnv(ConfigFileContent)))
			if err != nil {
				log.Fatalf("failed to load config: %v", err)
			}
		},
	})

	if os.Getenv("DISABLE_LOG") == "true" {
		log.SetOutput(io.Discard)
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		slog.SetDefault(logger)
	}

	response.SetSuccessResponseHandler(func(ctx *gin.Context, data any, statusCode int) {
		ctx.JSON(statusCode, data)
	})

	if facade.Config.GetBool("pyroscope.enabled") {
		go pyroscope2()
	}

	single := k8s.NewK8sClient()
	go func() {
		_, err := single.GetSdk().CreateServiceAccountSecret(single.GetSdk().GetServiceAccountName())
		if err != nil {
			slog.Warn("failed to create service account secret", "error", err)
		}
	}()

	httpServer := new(http.Provider).Register(newApp.GetConfig(), newApp.GetConsole(), newApp.GetServerManager()).Export()
	httpServer.Use(middleware.GetPanicHandlerMiddleware()).Use(commonmiddleware.HostCheck{}.Process)
	httpServer.RegisterRouters(func(engine *gin.Engine) {
		engine.Use(commonmiddleware.Cors{}.Process)
		engine.Use(commonmiddleware.Audit{}.Process)
		microappPath := facade.Config.GetString("static.microapp_path")
		if err := os.MkdirAll(microappPath, 0755); err != nil {
			slog.Error("failed to create microapp static directory", "path", microappPath, "error", err)
		}

		router := engine.Group("").
			Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4"}))).
			Use(cachecontrol.New(cachecontrol.CacheAssetsForeverPreset))
		routerNocache := engine.Group("/ui") //.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4"})))
		routerHtml := engine.Group("").
			Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedExtensions([]string{".pdf", ".mp4"}))).
			Use(cachecontrol.New(cachecontrol.NoCachePreset))

		staticPath := facade.Config.GetString("app.static_path")
		router.Static("/assets", staticPath+"/assets")
		router.Static("/longhorn", staticPath+"/longhorn")
		router.Static("/charts", staticPath+"/charts")
		router.Static("/schema", staticPath+"/schema")
		routerNocache.GET("/microapp/:identifie/:version/*path", controller.Static{}.FrontendProxy)
		router.Static("/ui/plugin", staticPath+"/plugin")
		router.Static("/ui/wasm", staticPath+"/wasm")
		router.Static("/ui/yaml", staticPath+"/yaml")

		routerHtml.StaticFileFS("/index.html", "index.html", stdhttp.FS(Asset))
		router.StaticFileFS("/k3s-agent.sh", "k3s-agent.sh", stdhttp.FS(Asset))
		router.StaticFileFS("/k3s-server.sh", "k3s-server.sh", stdhttp.FS(Asset))
		router.StaticFileFS("/favicon.ico", "icon.jpg", stdhttp.FS(Asset))
		router.StaticFileFS("/micro.html", "micro.html", stdhttp.FS(Asset))
		router.StaticFileFS("/logo.png", "logo.png", stdhttp.FS(Asset))
	})
	httpServer.RegisterRouters(
		func(engine *gin.Engine) {
			engine.Any("/k8s-proxy/*path",
				commonmiddleware.Auth{}.Process,
				commonmiddleware.K8sFilter{}.Process,
				controller.Proxy{}.ProxyK8s)
		},
	)

	new(application.Provider).Register(httpServer, newApp.GetConsole())
	new(auth.Provider).Register(httpServer, newApp.GetConsole())
	new(auditapp.Provider).Register(httpServer)
	new(metricsapp.Provider).Register(httpServer, newApp.GetConsole())
	new(zpk.Provider).Register(httpServer, newApp.GetConsole())
	new(k3k.Provider).Register(httpServer, newApp.GetConsole())
	new(k3sregistry.Provider).Register(httpServer, newApp.GetConsole())

	httpServer.RegisterRouters(func(engine *gin.Engine) {
		engine.NoRoute(commonmiddleware.Html{}.Process)
	})

	newApp.RunConsole()
}
