package permission

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	auditservice "github.com/w7panel/w7panel/common/service/audit"
)

type APIRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Verb        string `json:"verb"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func RoutesFromGin(routes gin.RoutesInfo) []APIRoute {
	seen := map[string]bool{}
	result := []APIRoute{}
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/panel-api/v1/") {
			continue
		}
		if strings.HasPrefix(route.Path, "/panel-api/v1/noauth/") {
			continue
		}
		verb := verbForHTTPMethod(route.Method)
		if verb == "" {
			continue
		}
		path := normalizeGinRoutePath(route.Path)
		description := routeDescription(route.Method, route.Path, path)
		item := APIRoute{
			Method:      route.Method,
			Path:        path,
			Verb:        verb,
			Title:       description,
			Description: description,
		}
		key := item.Method + " " + item.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Method < result[j].Method
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func routeDescription(method, rawPath string, normalizedPath string) string {
	if title := auditservice.LookupRouteDescription(method, rawPath); title != "" {
		return title
	}
	if title := auditservice.LookupRouteDescription(method, normalizedPath); title != "" {
		return title
	}
	return generatedRouteDescription(method, normalizedPath)
}

func normalizeGinRoutePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
}

func verbForHTTPMethod(method string) string {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return "get"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return ""
	}
}

func generatedRouteDescription(method string, path string) string {
	resource := routeResourceName(path)
	if resource == "" {
		return auditservice.MethodDescription(method)
	}
	if action := actionFromRoute(method, path); action != "" {
		return action + resource
	}
	return auditservice.MethodDescription(method) + resource
}

func routeResourceName(path string) string {
	path = strings.TrimPrefix(path, "/panel-api/v1/")
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	words := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "*" || strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			continue
		}
		for _, word := range strings.FieldsFunc(part, func(r rune) bool {
			return r == '-' || r == '_'
		}) {
			if translated := routeWordName(word); translated != "" {
				words = append(words, translated)
			}
		}
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " ")
}

func actionFromRoute(method string, path string) string {
	lowerPath := strings.ToLower(path)
	switch {
	case strings.Contains(lowerPath, "install"):
		return "安装"
	case strings.Contains(lowerPath, "uninstall"):
		return "卸载"
	case strings.Contains(lowerPath, "login"):
		return "登录"
	case strings.Contains(lowerPath, "sync"):
		return "同步"
	case strings.Contains(lowerPath, "download") || strings.Contains(lowerPath, "/down/"):
		return "下载"
	case strings.Contains(lowerPath, "upload"):
		return "上传"
	case strings.Contains(lowerPath, "import"):
		return "导入"
	case strings.Contains(lowerPath, "export"):
		return "导出"
	case strings.Contains(lowerPath, "verify"):
		return "校验"
	case strings.Contains(lowerPath, "reset"):
		return "重置"
	case strings.Contains(lowerPath, "refresh"):
		return "刷新"
	case strings.Contains(lowerPath, "proxy"):
		return "代理访问"
	case strings.Contains(lowerPath, "register"):
		return "注册"
	case strings.Contains(lowerPath, "delete"):
		return "删除"
	case strings.Contains(lowerPath, "create"):
		return "创建"
	case strings.Contains(lowerPath, "update") || strings.Contains(lowerPath, "patch"):
		return "更新"
	}
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return "获取"
	case "POST":
		return "提交"
	case "PUT":
		return "更新"
	case "PATCH":
		return "部分更新"
	case "DELETE":
		return "删除"
	default:
		return ""
	}
}

func routeWordName(word string) string {
	switch strings.ToLower(word) {
	case "api", "v1":
		return ""
	case "auth":
		return "认证"
	case "console":
		return "控制台"
	case "permission", "permissions":
		return "权限"
	case "route", "routes":
		return "路由列表"
	case "user", "userinfo":
		return "用户信息"
	case "login":
		return "登录"
	case "register":
		return "注册"
	case "token":
		return "Token"
	case "code":
		return "授权码"
	case "callback":
		return "回调"
	case "url", "uri":
		return "地址"
	case "oidc":
		return "OIDC"
	case "k3k":
		return "集群"
	case "namespace", "namespaces":
		return "命名空间"
	case "pod", "pods":
		return "Pod"
	case "service", "services":
		return "服务"
	case "proxy":
		return "代理"
	case "files", "file":
		return "文件"
	case "webdav":
		return "WebDAV"
	case "test":
		return "测试"
	case "gpu":
		return "GPU"
	case "config":
		return "配置"
	case "hami":
		return "HAMi"
	case "metrics":
		return "监控指标"
	case "real":
		return "实时"
	case "summary":
		return "汇总信息"
	case "node", "nodes":
		return "节点"
	case "device", "devices":
		return "设备列表"
	case "gpustack":
		return "GPUStack"
	case "worker":
		return "Worker"
	case "helm":
		return "Helm"
	case "release", "releases":
		return "应用发布"
	case "zpk":
		return "ZPK 应用"
	case "image":
		return "镜像"
	case "build", "buildimage":
		return "构建"
	case "job":
		return "Job"
	case "cronjob":
		return "定时 Job"
	case "static":
		return "静态资源"
	case "microapp":
		return "微应用"
	case "container", "containers":
		return "容器"
	case "registry":
		return "镜像仓库"
	case "dns":
		return "DNS"
	case "zone", "zones":
		return "Zone"
	case "record", "records":
		return "记录"
	case "longhorn":
		return "Longhorn"
	case "volume", "volumes":
		return "卷"
	case "snapshot":
		return "快照"
	case "s3bucket":
		return "S3 文件"
	default:
		return word
	}
}
