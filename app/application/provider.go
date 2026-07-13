package application

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	consoleShell "github.com/w7panel/w7panel/app/application/console"
	controller2 "github.com/w7panel/w7panel/app/application/http/controller"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/middleware"
	console2 "github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	appctl "github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/core"
	"github.com/w7panel/w7panel/common/service/k8s/gpu/gpustack"
	"github.com/w7panel/w7panel/common/service/k8s/higress"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	"github.com/w7panel/w7panel/common/service/k8s/shell"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/console"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	httpserver "github.com/we7coreteam/w7-rangine-go/v2/src/http/server"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Provider struct {
}

func (p Provider) Register(httpServer *httpserver.Server, console console.Console) {
	console.RegisterCommand(new(consoleShell.Goshell))
	console.RegisterCommand(new(consoleShell.K8sCheckResource))
	console.RegisterCommand(new(consoleShell.IngressUpgrade))
	console.RegisterCommand(new(consoleShell.MetricsInstall))
	console.RegisterCommand(new(consoleShell.UninstallStorePanel)) //删除商店安装的面板
	console.RegisterCommand(new(consoleShell.DomainParseConfig))   //域名解析
	console.RegisterCommand(new(consoleShell.SiteSettingUpgrade))  //旧站点配置升级
	console.RegisterCommand(new(consoleShell.UserUpgrade))         //旧用户升级
	console.RegisterCommand(new(consoleShell.W7ConfigUpgrade))     //旧 w7-config 升级
	console.RegisterCommand(new(consoleShell.PrivateDNSUpgrade))   //旧私有 DNS 升级
	console.RegisterCommand(new(consoleShell.Build))
	console.RegisterCommand(new(consoleShell.BeianCheck))      //备案检查
	console.RegisterCommand(new(consoleShell.TestUploadChunk)) // 测试分片上传功能

	p.RegisterHttpRoutes(httpServer)
	console2.SetConsoleApi(facade.GetConfig().GetString("app.console_base_url"))
	if helper.IsLocalMock() {
		// console2.SetConsoleApi("http://172.16.1.116:9004")
	}
	if err := p.syncSelfImageConfigMap(); err != nil {
		slog.Error("同步自有镜像配置失败", "error", err)
	}

	if facade.GetConfig().GetBool("longhorn.watch") {

		go longhorn.OnStart()
	}
	if facade.GetConfig().GetBool("k3sshell.enabled") {

		go shell.ShellWatch()
	}

	if facade.GetConfig().GetBool("clean.enabled") {

		go p.cleanS3()
	}

	if facade.GetConfig().GetBool("app.watch") {
		slog.Info("开始监听AppGroup资源变更事件")
		go appctl.Watch()
	}
	if facade.GetConfig().GetBool("gpustack.watch") {

		go gpustack.Watch()
	}

	if facade.GetConfig().GetBool("k8s.watch") {
		go core.StartControlManager()
	}

	go k8s.CheckLogo()
	go higress.LoadBkConfig()

}

func (p Provider) syncSelfImageConfigMap() error {
	const (
		namespace = "kube-system"
		name      = "w7panel-server"
	)

	repo, version := helper.SelfImageInfo()
	sdk := k8s.NewK8sClient()
	configMaps := sdk.ClientSet.CoreV1().ConfigMaps(namespace)
	ctx := context.Background()

	configMap, err := configMaps.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		_, err = configMaps.Create(ctx, &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string]string{
				"imageRepo":    repo,
				"imageVersion": version,
			},
		}, metav1.CreateOptions{})
		return err
	}

	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}

	if configMap.Data["imageRepo"] == repo && configMap.Data["imageVersion"] == version {
		return nil
	}

	configMap.Data["imageRepo"] = repo
	configMap.Data["imageVersion"] = version
	_, err = configMaps.Update(ctx, configMap, metav1.UpdateOptions{})
	return err
}

func (p Provider) RegisterHttpRoutes(server *httpserver.Server) {
	webdavMethods := []string{"PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "LINK", "UNLINK", "GET", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH", "POST"}
	server.RegisterRouters(func(engine *gin.Engine) {
		engine.GET("/docs/openapi", controller2.OpenAPI{}.Page)
		engine.GET("/docs/openapi/spec", controller2.OpenAPI{}.Spec)
		engine.GET("/openapi.json", controller2.OpenAPI{}.RedirectJSON)

		apiGroup := engine.Group("/panel-api/v1") //.Use(middleware.Cors{}.Process)
		{
			apiGroup.GET("/namespaces", middleware.Auth{}.Process, controller2.Namespaces{}.GetList)
			apiGroup.GET("/cluster/nodes/:name/longhorn-replicas", middleware.Auth{}.Process, controller2.Nodes{}.GetLonghornReplicas)
			apiGroup.POST("/cluster/nodes/:name/longhorn-replicas/delete", middleware.Auth{}.Process, controller2.Nodes{}.DeleteLonghornReplicas)
			apiGroup.DELETE("/cluster/nodes/:name", middleware.Auth{}.Process, controller2.Nodes{}.Delete)
			apiGroup.GET("/helm/releases", middleware.Auth{}.Process, controller2.Helm{}.List)
			apiGroup.GET("/helm/releases/:name", middleware.Auth{}.Process, controller2.Helm{}.Info)
			apiGroup.POST("/helm/releases/:name", middleware.Auth{}.Process, controller2.Helm{}.InstallUseRepo)
			apiGroup.DELETE("/helm/releases/:name", middleware.Auth{}.Process, controller2.Helm{}.UnInstall)
			apiGroup.PUT("/helm/releases/:name/reuse", middleware.Auth{}.Process, controller2.Helm{}.ReUseValues)
			apiGroup.GET("/app-info", controller2.Helm{}.AppInfo)
			apiGroup.GET("/test401", controller2.Helm{}.Test401)

		}

		localApiGroup := engine.Group("/panel-api/v1") //.Use(middleware.Cors{}.Process)
		{
			localApiGroup.GET("/tty", middleware.Auth{}.Process, controller2.PodExec{}.Tty)
			localApiGroup.GET("/nodetty", middleware.Auth{}.Process, controller2.PodExec{}.NodeTty)
			localApiGroup.GET("/download/*path", middleware.Auth{}.Process, controller2.File{}.Download)
			localApiGroup.POST("/cp", middleware.Auth{}.Process, controller2.PodExec{}.KubectlCp) //kubectl cp文件
			localApiGroup.POST("/cppid", middleware.Auth{}.Process, controller2.File{}.CpPidFile) //pid文件移动
			localApiGroup.POST("/mvpid", middleware.Auth{}.Process, controller2.File{}.CpPidFile) //pid文件移动

			localApiGroup.GET("/exec", middleware.Auth{}.Process, controller2.PodExec{}.Exec)
			localApiGroup.POST("/exec2", middleware.Auth{}.Process, controller2.PodExec{}.Exec)
			localApiGroup.POST("/exec-all", middleware.Auth{}.Process, controller2.PodExec{}.ExecAll)
			localApiGroup.GET("/pid", middleware.Auth{}.Process, middleware.CacheResponseWithExpire(time.Minute*1), controller2.Pid{}.GetPid) //获取所在pod和pid
			localApiGroup.GET("/mountfiles", middleware.Auth{}.Process, controller2.Pid{}.GetMountFiles)                                      //获取挂载文件配置                                     // 获取工作负载挂载文件
			localApiGroup.POST("/mountfiles", middleware.Auth{}.Process, controller2.Pid{}.CreateMountFile)                                   //新建挂载文件
			localApiGroup.PUT("/mountfiles", middleware.Auth{}.Process, controller2.Pid{}.UpdateMountFile)                                    //更新挂载文件列表                                      // 获取工作负载挂载文件
			localApiGroup.DELETE("/mountfiles", middleware.Auth{}.Process, controller2.Pid{}.DeleteMountFile)                                 //删除挂载文件
			localApiGroup.PUT("/mountfiles/chmod", middleware.Auth{}.Process, controller2.Pid{}.ChmodMountFile)                               //修改挂载文件权限
			localApiGroup.GET("/nodepid", middleware.Auth{}.Process, controller2.PodExec{}.GetNodePid)                                        //获取所在pod和pid

			localApiGroup.POST("/yaml", middleware.Auth{}.Process, controller2.Yaml{}.ApplyYamlOld)                // 直接提交yaml
			localApiGroup.PUT("/rollback", middleware.Auth{}.Process, controller2.Yaml{}.Rollback)                 // 回滚资源
			localApiGroup.POST("/kcompose", middleware.Auth{}.Process, controller2.Yaml{}.ConvertDockerComposeOld) // 转化kompose
			localApiGroup.POST("/pinyin", middleware.Auth{}.Process, controller2.Util{}.Pinyin)                    // pinyin
			localApiGroup.GET("/dnsip", middleware.Auth{}.Process, controller2.Util{}.DnsIp)
			localApiGroup.GET("/dns-cname", middleware.Auth{}.Process, controller2.Util{}.DnsCName)
			localApiGroup.GET("/myip", middleware.Auth{}.Process, controller2.Util{}.MyIp)
			localApiGroup.POST("/db-conn-test", middleware.Auth{}.Process, controller2.Util{}.DbConnTest) // 数据库连接测试
			localApiGroup.POST("/ping-etcd", middleware.Auth{}.Process, controller2.Util{}.PintEtcd)      // etcd连接测试
			localApiGroup.GET("/captcha", controller2.Util{}.Captcha)
			localApiGroup.POST("/verify-captcha", controller2.Util{}.VerifyCaptcha)
			for _, method := range webdavMethods {
				// 不转发到子pod
				localApiGroup.Handle(method, "/namespaces/:namespace/services/:name/proxy-root/*path", middleware.Auth{}.Process, controller2.Proxy{}.ProxyService)

				localApiGroup.Handle(method, "/namespaces/:namespace/services/:name/proxy/*path", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Proxy{}.ProxyService)
				localApiGroup.Handle(method, "/namespaces/:namespace/pods/:name/proxy/*path", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Proxy{}.ProxyPod)
				//代理转发
				localApiGroup.Handle(method, "/:name/proxy/*path", middleware.Auth{}.Process, controller2.Proxy{}.ProxyCommon) // 转发到子pod 文件管理需要访问agent 不能走proxy middleware
			}

			// localApiGroup.Any("/v1/:name/proxy/*path", controller2.Proxy{}.ProxyCommon)

			localApiGroup.Any("/proxy-url/", controller2.Proxy{}.ProxyAddr)

			localApiGroup.GET("/longhorn/need-delete-replica", middleware.Auth{}.Process, controller2.Longhorn{}.GetNeedDeleteReplicas)
			localApiGroup.GET("/longhorn/volumes/status", middleware.Auth{}.Process, controller2.Longhorn{}.GetVolumesStatus)
			localApiGroup.POST("/longhorn/install", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.Install)
			localApiGroup.POST("/longhorn/volumes/:volumeName/attach", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.Attach)
			localApiGroup.POST("/longhorn/volumes/:volumeName/detach", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.Detach)
			localApiGroup.POST("/longhorn/volumes/:volumeName/cancel-expansion", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.CancelExpansion)
			localApiGroup.POST("/longhorn/volumes/:volumeName/trim-filesystem", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.TrimFilesystem)
			localApiGroup.POST("/longhorn/volumes/:volumeName/snapshot-delete", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.SnapshotDelete)
			localApiGroup.POST("/longhorn/volumes/:volumeName/snapshot-purge", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Longhorn{}.SnapshotPurge)

			localApiGroup.GET("/static/:identifie/status", middleware.Auth{}.Process, controller2.Static{}.StaticInfo)
			localApiGroup.POST("/static/:namespace/download/:name", middleware.Auth{}.Process, controller2.Static{}.Download)
			// 前端静态资源回源代理：本地未下载时从远程制品库拉取
			localApiGroup.GET("/static/proxy/:zpkUrl/:identifie/:version/frontend/*path", controller2.Static{}.FrontendProxy)

		}
		gpuGroup := engine.Group("/panel-api/v1/gpu").Use(middleware.Auth{}.Process, middleware.Proxy{}.Process)
		{
			gpuGroup.POST("/enabled-gpu", controller2.Gpu{}.EnableGpu)                   // 开启关闭gpu
			gpuGroup.POST("/install-hami", controller2.Gpu{}.InstallHami)                // 安装hami
			gpuGroup.POST("/install-gpu-operator", controller2.Gpu{}.InstallGpuOperator) // 安装gpu-op
			gpuGroup.GET("/config", controller2.Gpu{}.GetGpuConfig)                      // 获取配置
			gpuGroup.GET("/hami/metrics/real", controller2.Gpu{}.HamiMetricsReal)        // hami实时监控利用率百分比
			gpuGroup.GET("/summary", controller2.Gpu{}.GpuSummary)
			gpuGroup.GET("/node/devices", controller2.Gpu{}.NodesDevices)
			gpuGroup.POST("/gpustack/worker", controller2.Gpu{}.CreateGpuStackWorker)
		}
		for _, method := range webdavMethods {
			// engine.Handle(method, "/panel-api/v1/files/webdav-agent/:pid/subagent/:subpid/agent/*path", middleware.Auth{}.Process, controller2.Webdav{}.HandlePidSubPid)

			engine.Handle(method, "/panel-api/v1/files/webdav-agent/:pid/agent/*path", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Webdav{}.HandlePid)
			// engine.Handle(method, "/panel-api/v1/files/webdav-test/*path", controller2.Webdav{}.HandleTest)
		}
		// /etc/passwd 缓存

		// 新版 API - 代理到服务 //TODO 没有权限校验的代理接口，需要加auth middleware
		engine.Any("/panel-api/v1/namespaces/:namespace/services/:name/proxy-no/*path", middleware.ProxyNoAuth{}.Process, controller2.Proxy{}.ProxyNoAuthService)

		engine.POST("/panel-api/v1/files/compress-agent/:pid/compress", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.CompressAgent{}.Compress)
		engine.POST("/panel-api/v1/files/compress-agent/:pid/extract", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.CompressAgent{}.Extract)
		// engine.POST("/panel-api/v1/files/compress-agent/:pid/subagent/:subpid/compress", middleware.Auth{}.Process, controller2.CompressAgent{}.Compress)
		// engine.POST("/panel-api/v1/files/compress-agent/:pid/subagent/:subpid/extract", middleware.Auth{}.Process, controller2.CompressAgent{}.Extract)

		engine.POST("/panel-api/v1/files/permission-agent/:pid/chmod", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.PermissionAgent{}.Chmod)
		engine.POST("/panel-api/v1/files/permission-agent/:pid/chown", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.PermissionAgent{}.Chown)
		// engine.POST("/panel-api/v1/files/permission-agent/:pid/subagent/:subpid/chmod", middleware.Auth{}.Process, controller2.PermissionAgent{}.Chmod)
		// engine.POST("/panel-api/v1/files/permission-agent/:pid/subagent/:subpid/chown", middleware.Auth{}.Process, controller2.PermissionAgent{}.Chown)

		// 分片上传相关接口
		localApiGroup.POST("/files/upload-chunk", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.File{}.UploadChunk) // 上传分片
		localApiGroup.GET("/files/check-chunk", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.File{}.CheckChunk)    // 检查分片是否已上传
		localApiGroup.POST("/files/merge-chunks", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.File{}.MergeChunks) // 合并分片
		// localApiGroup.POST("/files/mvtopod", middleware.Auth{}.Process, middleware.Proxy{}.Process, middleware.Auth{}.Process, controller2.File{}.MoveToPod) //pid文件移动

		engine.GET("/panel-api/v1/kubeconfig", middleware.Auth{}.Process, middleware.Proxy{}.Process, controller2.Proxy{}.Kubeconfig)
		engine.Any("/panel-api/v1/s3bucket", middleware.Auth{}.Process, controller2.File{}.Upload).Use(middleware.Cors{}.Process)
		engine.Any("s3bucket", middleware.Auth{}.Process, controller2.File{}.Upload).Use(middleware.Cors{}.Process) //s3fakeserver 不支持多路径

		// 安全的未授权接口 - 只返回必要的公开字段 加1分钟缓存
		engine.GET("/panel-api/v1/noauth/site/beian", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.Beian)
		engine.GET("/panel-api/v1/noauth/site/beian2", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.Beian2)
		engine.GET("/panel-api/v1/noauth/site/k3k-config", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.K3kConfig)
		engine.GET("/panel-api/v1/noauth/site/init-user", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.InitUser)
		engine.GET("/panel-api/v1/noauth/site/lianxi", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.Lianxi)
		// 镜像源配置文件
		engine.GET("/panel-api/v1/noauth/site/registries", controller2.Site{}.Registries)
		// 获取配置文件内容，label w7.cc/noauth=true 允许不授权访问
		// 用于应用直达 注册协议 和logo 通用接口
		engine.GET("/panel-api/v1/noauth/site/{name}/configmap", middleware.CacheResponseWithExpire(time.Minute*1), controller2.Site{}.NoAuthConfigMap)

		engine.GET("/panel-api/v1/microapp/top", middleware.Auth{}.Process, controller2.MicroApp{}.List)                     //获取microapp列表
		engine.GET("/panel-api/v1/microapp/:name/info", middleware.Auth{}.Process, controller2.MicroApp{}.Info)              //获取microapp详情
		engine.Any("/panel-api/v1/microapp/:name/proxy/*path", middleware.Auth{}.Process, controller2.Proxy{}.ProxyMicroApp) //microapp proxy

		engine.GET("/panel-api/v1/microapp/:name/frontprops", middleware.Auth{}.Process, controller2.MicroApp{}.FrontProps)
		engine.GET("/panel-api/v1/microapp/global-frontprops", middleware.Auth{}.Process, controller2.MicroApp{}.GlobalFrontProps) //获取microapp前端系统参数

		containerGroup := localApiGroup.Group("/containers", middleware.Auth{}.Process, middleware.Proxy{}.Process)
		{
			containerGroup.POST("/image/export-push", controller2.Container{}.ExportAndPushImage)
		}

	})
}

func (p Provider) cleanS3() {
	sen := facade.Config.GetDuration("clean.interval")
	ticker := time.NewTicker(sen)
	quit := make(chan struct{})

	for {
		select {
		case <-quit:
			ticker.Stop()
			return

		case <-ticker.C:
			s3dir := facade.Config.GetString("s3.base_dir")
			err := os.RemoveAll(s3dir + "/upload")
			if err != nil {
				slog.Error("clean s3 error", "err", err)
			}
		}

	}
}
