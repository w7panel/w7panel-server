package controller

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	netpath "path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Static struct {
	controller.Abstract
}

const frontendSourceHeader = "X-Frontend-Source"

func (self Static) StaticInfo(http *gin.Context) {
	identifie := http.Param("identifie")
	version := http.Query("version")
	releaseName := http.Query("releaseName")
	if strings.Contains(releaseName, "-root") {
		releaseName = strings.ReplaceAll(releaseName, "-root", "")
	}
	status := appgroup.DownStaticStatus(identifie, version, releaseName)

	// 通过 releaseName 查找 AppGroup，获取 zpkUrl 和 ticket 信息，并缓存 ticket
	zpkUrl := ""
	ticket := ""
	proxyUrl := "/ui/microapp/" + identifie + "/" + version + "/index.html"
	if releaseName != "" && releaseName != "default" {
		sdk := k8s.NewK8sClient().Sdk
		group, err := appgroup.GetAppgroupUseSdk(releaseName, "default", sdk)
		if err == nil {
			// 去掉 path 部分，只保留 scheme://host
			if parsedUrl, parseErr := url.Parse(group.Spec.ZpkUrl); parseErr == nil {
				parsedUrl.Path = ""
				parsedUrl.RawPath = ""
				parsedUrl.RawQuery = ""
				parsedUrl.Fragment = ""
				zpkUrl = parsedUrl.String()
			} else {
				zpkUrl = group.Spec.ZpkUrl
			}
			if group.Annotations != nil {
				ticket = group.Annotations["w7.cc/ticket"]
			}
			// 缓存 ticket 供 FrontendProxy 使用
			if ticket != "" {
				helper.Set("frontend-ticket-"+identifie, ticket, time.Hour*2)
			}
			// 缓存 zpkUrl 供 FrontendProxy 使用
			if zpkUrl != "" {
				helper.Set("frontend-zpk-url-"+identifie, zpkUrl, time.Hour*2)
			}
		}
	}
	if status == appgroup.DOWNLOAD_SUCCESS {
		proxyUrl = "/ui/microapp/" + identifie + "/" + version + "/index.html"
	}

	self.JsonResponseWithoutError(http, gin.H{
		"status":   status,
		"proxyUrl": proxyUrl,
		"zpkUrl":   zpkUrl,
		// "ticket":   ticket,
	})
}

func (self Static) Download(http *gin.Context) {
	name := http.Param("name")
	namespace := http.Param("namespace")
	token := http.MustGet("k8s_token").(string)

	rootSdk := k8s.NewK8sClient().Sdk
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	useSdk := sdk
	hasRoot := strings.Contains(name, "-root")
	if hasRoot {
		name = strings.ReplaceAll(name, "-root", "")
		useSdk = rootSdk
	}
	appgroupObj, err := appgroup.GetAppgroupUseSdk(name, namespace, useSdk)
	if err != nil {
		// 尝试从root集群获取
		if apierrors.IsNotFound(err) {
			group, err := appgroup.GetAppgroupUseSdk(name, namespace, rootSdk)
			if err != nil {
				self.JsonResponseWithServerError(http, err)
				return
			}
			appgroupObj = group
		} else {
			self.JsonResponseWithServerError(http, err)
			return
		}
	}
	appgroup.DownStatic(appgroupObj)

}

// FrontendProxy 代理前端静态资源请求到远程制品库
// zpkUrl 和 ticket 从缓存获取
// URL: /ui/microapp/:identifie/:version/*path
// URL: /panel-api/v1/static/proxy/:zpkUrl/:identifie/:version/frontend/*path
// 代理到: {zpkUrl}/zpk/respo/attach/frontend/{identifie}/{version}/{path}?ticket={ticket}
func (self Static) FrontendProxy(ctx *gin.Context) {
	zpkUrlEncoded := ctx.Param("zpkUrl")
	identifie := ctx.Param("identifie")
	version := ctx.Param("version")
	path := ctx.Param("path")

	if identifie == "" || version == "" {
		self.JsonResponseWithServerError(ctx, fmt.Errorf("identifie and version are required"))
		return
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	ignoreLocal := os.Getenv("STATIC_IGNORE_LOCAL") == "true"

	if appgroup.DownStaticStatus(identifie, version, "") == appgroup.DOWNLOAD_SUCCESS && !ignoreLocal {
		microappPath := facade.Config.GetString("static.microapp_path")
		localFile, found, err := resolveMicroappLocalFile(microappPath, identifie, version, path)
		if err != nil {
			slog.Warn("invalid microapp local file request", "identifie", identifie, "version", version, "path", path, "error", err)
			ctx.String(http.StatusBadRequest, "invalid microapp path")
			return
		}
		if !found {
			ctx.String(http.StatusNotFound, "microapp file not found")
			return
		}
		ctx.Header(frontendSourceHeader, "local")
		// // 客户端支持 gzip 时压缩后返回，减少传输体积
		// if strings.Contains(ctx.GetHeader("Accept-Encoding"), "gzip") {
		// 	data, err := os.ReadFile(localFile)
		// 	if err != nil {
		// 		slog.Error("读取本地文件失败", "path", localFile, "error", err)
		// 		ctx.String(http.StatusInternalServerError, err.Error())
		// 		return
		// 	}
		// 	var buf bytes.Buffer
		// 	gz := gzip.NewWriter(&buf)
		// 	if _, err := gz.Write(data); err != nil {
		// 		slog.Error("gzip压缩失败", "error", err)
		// 		ctx.String(http.StatusInternalServerError, err.Error())
		// 		return
		// 	}
		// 	gz.Close()
		// 	ctx.Header("Content-Encoding", "gzip")
		// 	ctx.Data(http.StatusOK, http.DetectContentType(data), buf.Bytes())
		// 	return
		// }
		ctx.File(localFile)
		return
	}

	// 从缓存获取 zpkUrl，兼容旧路径中携带 base64url zpkUrl 的方式
	zpkUrl := ""
	if val, ok := helper.Get("frontend-zpk-url-" + identifie); ok {
		if cachedZpkUrl, ok := val.(string); ok {
			zpkUrl = cachedZpkUrl
		}
	}
	if zpkUrl == "" && zpkUrlEncoded != "" {
		zpkUrlBytes, err := base64.RawURLEncoding.DecodeString(zpkUrlEncoded)
		if err != nil {
			slog.Error("解码zpkUrl失败", "zpkUrlEncoded", zpkUrlEncoded, "error", err)
			self.JsonResponseWithServerError(ctx, err)
			return
		}
		zpkUrl = string(zpkUrlBytes)
	}
	zpkUrl = strings.TrimRight(zpkUrl, "/")
	if zpkUrl == "" {
		slog.Error("zpkUrl为空")
		ctx.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "zpkUrl is empty"})
		return
	}

	// 从缓存获取 ticket
	ticket := ""
	if val, ok := helper.Get("frontend-ticket-" + identifie); ok {
		ticket = val.(string)
	}

	// 构造远程 URL
	remotePath := fmt.Sprintf("/zpk/respo/attach/frontend/%s/%s%s", identifie, version, path)
	remoteUrlStr := zpkUrl + remotePath
	if ticket != "" {
		remoteUrlStr += "?ticket=" + url.QueryEscape(ticket)
	}

	remoteUrl, err := url.Parse(remoteUrlStr)
	if err != nil {
		slog.Error("解析远程URL失败", "url", remoteUrlStr, "error", err)
		self.JsonResponseWithServerError(ctx, err)
		return
	}

	slog.Info("前端资源回源代理",
		"identifie", identifie,
		"version", version,
		"path", path,
		"remote", remoteUrlStr,
	)

	// 阻止后续中间件
	ctx.Abort()

	proxy := httputil.NewSingleHostReverseProxy(remoteUrl)
	proxy.Director = func(req *http.Request) {
		req.Host = remoteUrl.Host
		req.URL.Scheme = remoteUrl.Scheme
		req.URL.Host = remoteUrl.Host
		req.URL.Path = remotePath
		req.URL.RawPath = ""
		acceptEncoding := req.Header.Get("Accept-Encoding")
		if acceptEncoding != "" {
			req.Header.Del("Accept-Encoding")
			req.Header.Add("Accept-Encoding", "gzip")
		}

		if ticket != "" {
			req.URL.RawQuery = "ticket=" + url.QueryEscape(ticket)
		} else {
			req.URL.RawQuery = ""
		}
	}
	proxy.ModifyResponse = func(res *http.Response) error {
		res.Header.Del("Access-Control-Allow-Origin")
		res.Header.Set(frontendSourceHeader, "proxy")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("前端资源代理错误", "error", err, "path", r.URL.Path)
		w.Header().Set(frontendSourceHeader, "proxy")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":502,"error":"%s"}`, err.Error())))
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in frontend proxy", "error", r)
		}
	}()

	proxy.ServeHTTP(ctx.Writer, ctx.Request)
}

func resolveMicroappLocalFile(microappPath, identifie, version, requestPath string) (string, bool, error) {
	baseDir := filepath.Join(microappPath, identifie, version)
	localPath, err := cleanMicroappRequestPath(requestPath)
	if err != nil {
		return "", false, err
	}
	if localPath == "" {
		localPath = "index.html"
	}

	target, err := joinMicroappLocalPath(baseDir, localPath)
	if err != nil {
		return "", false, err
	}
	if !microappFileExists(target) {
		return "", false, nil
	}

	return target, true, nil
}

func cleanMicroappRequestPath(requestPath string) (string, error) {
	for _, segment := range strings.Split(strings.ReplaceAll(requestPath, "\\", "/"), "/") {
		if segment == ".." {
			return "", fmt.Errorf("path contains parent directory segment")
		}
	}
	cleanPath := netpath.Clean("/" + requestPath)
	return strings.TrimPrefix(cleanPath, "/"), nil
}

func joinMicroappLocalPath(baseDir, localPath string) (string, error) {
	if strings.Contains(localPath, "\x00") {
		return "", fmt.Errorf("path contains NUL byte")
	}

	target := filepath.Join(baseDir, filepath.FromSlash(localPath))
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes microapp directory")
	}

	return targetAbs, nil
}

func microappFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
