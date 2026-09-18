package controller

import (
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	zpkcontroller "github.com/w7panel/w7panel/app/zpk/http"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/microapp"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	"github.com/w7panel/w7panel/common/service/oidc"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Proxy struct {
	controller.Abstract
}

func (self Proxy) ProxyK8s(http *gin.Context) {
	// 提取并归一化路径：移除 /k8s-proxy 或 /k8s 前缀
	path := http.Param("path")
	if path == "" {
		path = http.Request.URL.Path
	}
	// 移除前缀
	if strings.HasPrefix(path, "/k8s-proxy") {
		path = strings.TrimPrefix(path, "/k8s-proxy")
	} else if strings.HasPrefix(path, "/k8s") {
		path = strings.TrimPrefix(path, "/k8s")
	}
	if path == "" {
		path = "/"
	}
	if self.proxyPanelAPI(http, path) {
		return
	}

	// 修改请求路径
	http.Request.URL.Path = path
	http.Request.URL.RawPath = ""

	// 获取 token
	token := http.MustGet("k8s_token").(string)
	local := http.Query("local")
	forceLocal := false
	if local == "true" || local == "1" {
		forceLocal = true
	}
	// 创建 K8s 客户端
	client, err := k8s.NewK8sClient().ChannelLocal(token, forceLocal)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	// 检查并修改 http.Request 中的 Authorization 头部
	auth := http.Request.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		// bearerToken := strings.TrimPrefix(auth, "Bearer ")
		restConfig, err := client.ToRESTConfig()
		if err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
		http.Request.Header.Set("Authorization", "Bearer "+restConfig.BearerToken)
		// if bearerToken != token {
		// 	// 如果 Bearer 令牌与 token 不一致，修改 Authorization 头部
		// 	http.Request.Header.Set("Authorization", "Bearer "+token)
		// }
	} else if auth != "" {
		// 如果 Authorization 头部不是 Bearer 类型，考虑是否需要清除或处理
		// 根据实际需求决定，这里假设我们清除它并使用 token
		http.Request.Header.Set("Authorization", "Bearer "+token)
	}

	err = client.Proxy(http.Request, http.Writer)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
}

// proxyPanelAPI exposes panel's Kubernetes-backed operations below the same
// credential boundary as the native Kubernetes proxy. New endpoints belong
// here; /panel-api must not accept a caller supplied Kubernetes token.
func (self Proxy) proxyPanelAPI(http *gin.Context, path string) bool {
	if path == "/panel/v1/helm/releases" && http.Request.Method == stdhttp.MethodGet {
		Helm{}.List(http)
		return true
	}
	if strings.HasPrefix(path, "/panel/v1/helm/releases/") {
		http.Params = append(http.Params, gin.Param{Key: "name", Value: strings.TrimPrefix(path, "/panel/v1/helm/releases/")})
		switch http.Request.Method {
		case stdhttp.MethodGet:
			Helm{}.Info(http)
		case stdhttp.MethodPost:
			Helm{}.InstallUseRepo(http)
		case stdhttp.MethodDelete:
			Helm{}.UnInstall(http)
		case stdhttp.MethodPut:
			if strings.HasSuffix(path, "/reuse") {
				http.Params[len(http.Params)-1].Value = strings.TrimSuffix(strings.TrimPrefix(path, "/panel/v1/helm/releases/"), "/reuse")
				Helm{}.ReUseValues(http)
				return true
			}
			return false
		default:
			return false
		}
		return true
	}
	if path == "/panel/v1/zpk/upgrade-info" && http.Request.Method == stdhttp.MethodGet {
		zpkcontroller.Zpk{}.UpgradeInfo(http)
		return true
	}
	return false
}

func (self Proxy) ProxyNoAuthService(gin *gin.Context) {
	self.ProxyService(gin)
}

// 转发k8s
func (self Proxy) ProxyService(gin *gin.Context) {
	ns := gin.Param("namespace")
	name := gin.Param("name")
	path := gin.Param("path")
	if path == "" {
		path = "/"
	}

	//name split : port
	schema, host, port := self.parseName(name)
	cdomain := helper.ClusterDomain(host, ns)
	proxyUrl := schema + "://" + cdomain + ":" + port

	if facade.GetConfig().GetBool("longhorn.mock") {
		// proxyUrl = "http://218.23.2.55:9090/"
	}
	// token := gin.GetString("k8s_token")
	// if token != "" { //proxy-no 不传递token
	// 	k8sToken := k8s.NewK8sToken(token)
	// 	if k8sToken.IsVirtual() {
	// 		client, err := k8s.NewK8sClient().Channel(token)
	// 		if err != nil {
	// 			self.JsonResponseWithServerError(gin, err)
	// 			return
	// 		}
	// 		client.Proxy(gin.Request, gin.Writer)
	// 		return
	// 	}
	// }

	self.proxyUrl(gin, proxyUrl, path)
}

func (self Proxy) ProxyPod(gin *gin.Context) {
	ns := gin.Param("namespace")
	name := gin.Param("name")
	path := gin.Param("path")
	if path == "" {
		path = "/"
	}
	//name split : port
	namePort := strings.Split(name, ":")
	podName := namePort[0]
	podPort := "80"
	if len(namePort) > 1 {
		podPort = namePort[1]
	}

	client, err := k8s.NewK8sClient().Channel(gin.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	pod, err := client.ClientSet.CoreV1().Pods(ns).Get(client.Ctx, podName, v1.GetOptions{})
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	podId := pod.Status.PodIP
	if podId == "" {
		self.JsonResponseWithServerError(gin, errors.New("pod ip is empty"))
		return
	}

	proxyUrl := "http://" + podId + ":" + podPort

	self.proxyUrl(gin, proxyUrl, path)
}

func (self Proxy) ProxyCommon(gin *gin.Context) {

	name := gin.Param("name")
	path := gin.Param("path")
	if path == "" {
		path = "/"
	}
	schema, host, port := self.parseName(name)

	proxyUrl := schema + "://" + host + ":" + port
	slog.Info("proxyUrl:" + proxyUrl)

	self.proxyUrl(gin, proxyUrl, path)
}

func (self Proxy) proxyUrl(gin *gin.Context, proxyUrl string, path string) {
	remote, err := url.Parse(proxyUrl)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	proxyPath := remote.Path
	if path == "" {
		path = proxyPath
	}

	// 标记已处理，阻止后续中间件
	gin.Abort()

	proxy := httputil.NewSingleHostReverseProxy(remote)
	proxy.Director = func(req *stdhttp.Request) {
		req.Host = remote.Host
		req.URL.Scheme = remote.Scheme
		req.URL.Host = remote.Host
		req.URL.Path = path

		// 处理 WebDAV Destination 头
		if dest := req.Header.Get("Destination"); dest != "" {
			if destURL, err := url.Parse(dest); err == nil {
				destPath := destURL.Path
				if idx := strings.Index(destPath, "/panel-api/v1/files/webdav"); idx >= 0 {
					destPath = destPath[idx:]
				} else if idx := strings.Index(destPath, "/webdav"); idx >= 0 {
					destPath = destPath[idx:]
				}
				if destURL.RawQuery != "" {
					destPath += "?" + destURL.RawQuery
				}
				req.Header.Set("Destination", destPath)
				slog.Info("Rewrote Destination header", "original", dest, "new", destPath)
			}
		}
	}
	proxy.ModifyResponse = func(res *stdhttp.Response) error {
		res.Header.Del("Access-Control-Allow-Origin")
		return nil
	}
	proxy.ErrorHandler = func(w stdhttp.ResponseWriter, r *stdhttp.Request, err error) {
		slog.Error("Proxy error", "error", err, "path", r.URL.Path)
		w.WriteHeader(stdhttp.StatusBadGateway)
		w.Write([]byte(fmt.Sprintf(`{"code":502,"error":"%s"}`, err.Error())))
	}

	// 使用 defer recover 捕获可能的 panic
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in proxy", "error", r)
		}
	}()

	proxy.ServeHTTP(gin.Writer, gin.Request)
}

func (self Proxy) ProxyAddr(http *gin.Context) {
	type ParamsValidate struct {
		ProxyUrl string `form:"proxyUrl" binding:"required`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	res, err := helper.RetryHttpClient().R().Get(params.ProxyUrl)
	if err != nil {
		http.String(200, "")
		return
	}
	http.String(200, res.String())
	// res.Body().Close()
	// uri, err := url.Parse(params.ProxyUrl)
	// if err != nil {
	// 	self.JsonResponseWithServerError(http, err)
	// 	return
	// }

	// self.proxyUrl(http, params.ProxyUrl, "")
}

func (self Proxy) Kubeconfig(gin *gin.Context) {
	apiServerUrl := gin.Query("apiServerUrl")
	config, err := k8s.NewK8sClient().ToKubeconfigForServiceAccount(apiServerUrl, permissionservice.APIPermissionName)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	self.JsonResponseWithoutError(gin, config)

}

func (self Proxy) parseName(name string) (string, string, string) {
	namePort := strings.Split(name, ":")
	host := namePort[0]
	port := "80"
	schema := "http"
	if len(namePort) == 3 {
		schema = namePort[0]
		host = namePort[1]
		port = namePort[2]
	}
	if len(namePort) == 2 {
		host = namePort[0]
		port = namePort[1]
	}
	if len(namePort) == 1 {
		host = namePort[0]
	}
	return schema, host, port
}

func (self Proxy) ProxyMicroApp(gin *gin.Context) {

	name := gin.Param("name")
	path := gin.Param("path")
	clientHost := gin.Request.Host

	token := gin.MustGet("k8s_token").(string)
	role := k8s.NewK8sToken(token).GetRole()
	microAppObj, err := microapp.ListInfo(token, name)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	proxy := microapp.NewMicroAppProxy(microAppObj, false, role)
	if replace, err := microapp.NewMicroAppReplace(token); err == nil && replace != nil {
		proxy.WithReplace(replace)
	}
	proxyCtx := gin.Request.Context()
	if server, err := oidc.GetServer(); err == nil && server != nil {
		proxyCtx = server.ContextWithIssuer(proxyCtx, gin.Request)
	}
	revert, err := proxy.Proxy(proxyCtx, path)
	if err != nil {
		self.JsonResponseWithServerError(gin, err)
		return
	}
	// The microapp proxy normally changes Host to the internal Service name.
	// A WebSocket browser request keeps its public Origin, so that rewrite makes
	// the target's default same-origin check reject the upgrade with 403. Keep
	// the client Host only for upgrades; the transport still targets ServerUrl.
	if strings.EqualFold(gin.GetHeader("Upgrade"), "websocket") && clientHost != "" {
		director := revert.Director
		revert.Director = func(req *stdhttp.Request) {
			director(req)
			req.Host = clientHost
		}
	}
	revert.ServeHTTP(gin.Writer, gin.Request)
}
