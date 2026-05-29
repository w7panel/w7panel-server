package audit

import "strings"

var routeDescriptions = map[string]string{
	"POST /panel-api/v1/login": "用户登录",

	"POST /panel-api/v1/oidc/register":             "注册 OIDC 客户端",
	"PUT /panel-api/v1/oidc/register/:clientId":    "更新 OIDC 客户端",
	"DELETE /panel-api/v1/oidc/register/:clientId": "删除 OIDC 客户端",
	"POST /panel-api/v1/oidc/authorize/login":      "OIDC 授权登录",
	"POST /panel-api/v1/oidc/js-code":              "获取 OIDC 授权码",
	"POST /panel-api/v1/oidc/redirect-uri":         "获取 OIDC 回调地址",
	"* /panel-api/v1/oidc/authorize":               "处理 OIDC 授权请求",
	"* /panel-api/v1/oidc/token":                   "处理 OIDC Token 请求",
	"* /panel-api/v1/oidc/userinfo":                "处理 OIDC 用户信息请求",

	"POST /panel-api/v1/auth/login":                       "控制台签名登录",
	"POST /panel-api/v1/auth/register":                    "注册用户",
	"POST /panel-api/v1/auth/refresh-token2":              "刷新登录令牌",
	"POST /panel-api/v1/auth/init-user":                   "初始化用户",
	"POST /panel-api/v1/auth/reset-password":              "重置用户密码",
	"POST /panel-api/v1/auth/reset-password-current":      "修改当前用户密码",
	"* /panel-api/v1/auth/console/proxy/*path":            "代理控制台请求",
	"POST /panel-api/v1/auth/console/register-to-console": "注册到控制台",
	"POST /panel-api/v1/auth/console/thirdparty-cd-token": "获取第三方 CD Token",
	"POST /panel-api/v1/auth/console/import-cert":         "导入证书",
	"POST /panel-api/v1/auth/console/verify-cert":         "校验证书",
	"POST /panel-api/v1/auth/console/import-cert-console": "导入控制台证书",
	"POST /panel-api/v1/auth/console/register-zpk-site":   "注册 ZPK 站点",
	"POST /panel-api/v1/code":                             "获取授权码",
	"POST /panel-api/v1/callback-url":                     "获取回调地址",

	"* /v2/*path": "处理镜像仓库请求",
	"POST /panel-api/v1/registry/containers/:id/commit": "提交容器为镜像",
	"PUT /panel-api/v1/registry/patch/images/tag":       "修改镜像标签",
	"POST /panel-api/v1/registry/patch/images/delete":   "删除镜像",
	"POST /panel-api/v1/registry/patch/images/label":    "修改镜像标签信息",
	"POST /panel-api/v1/registry/patch/images/import":   "导入镜像",

	"POST /panel-api/v1/k3k/init":                              "初始化集群",
	"POST /panel-api/v1/k3k/sync-ingress":                      "同步 Ingress",
	"POST /panel-api/v1/k3k/sync-configmap":                    "同步 ConfigMap",
	"POST /panel-api/v1/k3k/sync-mcpbridge":                    "同步 MCP Bridge",
	"POST /panel-api/v1/k3k/sync-secret":                       "同步 Secret",
	"POST /panel-api/v1/k3k/sync-down-static":                  "同步静态下载资源",
	"POST /panel-api/v1/k3k/sync-microapp":                     "同步微应用",
	"POST /panel-api/v1/k3k/login":                             "登录主集群",
	"POST /panel-api/v1/k3k/order/notify":                      "处理订单通知",
	"POST /panel-api/v1/k3k/order/license":                     "创建面板授权订单",
	"POST /panel-api/v1/k3k/cvm/:namespace/action/:name/login": "登录 CVM",
	"POST /k8s/k3k/order/notify":                               "处理旧版订单通知",

	"POST /panel-api/v1/helm/releases/:name":      "安装 Helm 应用",
	"DELETE /panel-api/v1/helm/releases/:name":    "卸载 Helm 应用",
	"PUT /panel-api/v1/helm/releases/:name/reuse": "复用 Helm 配置",

	"POST /panel-api/v1/cp":                                                 "复制 Pod 文件",
	"POST /panel-api/v1/cppid":                                              "复制 PID 文件",
	"POST /panel-api/v1/mvpid":                                              "移动 PID 文件",
	"POST /panel-api/v1/exec2":                                              "执行命令",
	"POST /panel-api/v1/exec-all":                                           "批量执行命令",
	"POST /panel-api/v1/mountfiles":                                         "新建挂载文件",
	"PUT /panel-api/v1/mountfiles":                                          "更新挂载文件",
	"DELETE /panel-api/v1/mountfiles":                                       "删除挂载文件",
	"PUT /panel-api/v1/mountfiles/chmod":                                    "修改挂载文件权限",
	"POST /panel-api/v1/yaml":                                               "提交 YAML",
	"PUT /panel-api/v1/rollback":                                            "回滚资源",
	"POST /panel-api/v1/kcompose":                                           "转换 Docker Compose",
	"POST /panel-api/v1/pinyin":                                             "转换拼音",
	"POST /panel-api/v1/dns/zones":                                          "创建 DNS Zone",
	"DELETE /panel-api/v1/dns/zones/:domain":                                "删除 DNS Zone",
	"POST /panel-api/v1/dns/zones/:domain/records":                          "创建 DNS 记录",
	"PUT /panel-api/v1/dns/zones/:domain/records/:id":                       "更新 DNS 记录",
	"DELETE /panel-api/v1/dns/zones/:domain/records/:id":                    "删除 DNS 记录",
	"PUT /panel-api/v1/dns/server":                                          "更新 DNS 服务配置",
	"POST /panel-api/v1/db-conn-test":                                       "测试数据库连接",
	"POST /panel-api/v1/ping-etcd":                                          "测试 etcd 连接",
	"POST /panel-api/v1/verify-captcha":                                     "校验验证码",
	"* /panel-api/v1/namespaces/:namespace/services/:name/proxy-root/*path": "代理访问服务根路径",
	"* /panel-api/v1/namespaces/:namespace/services/:name/proxy/*path":      "代理访问服务",
	"* /panel-api/v1/namespaces/:namespace/pods/:name/proxy/*path":          "代理访问 Pod",
	"* /panel-api/v1/:name/proxy/*path":                                     "代理访问组件",
	"* /panel-api/v1/proxy-url/":                                            "代理访问 URL",

	"POST /panel-api/v1/longhorn/install":                              "安装 Longhorn",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/attach":           "挂载 Longhorn 卷",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/detach":           "卸载 Longhorn 卷",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/cancel-expansion": "取消 Longhorn 卷扩容",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/trim-filesystem":  "修剪 Longhorn 文件系统",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/snapshot-delete":  "删除 Longhorn 快照",
	"POST /panel-api/v1/longhorn/volumes/:volumeName/snapshot-purge":   "清理 Longhorn 快照",
	"POST /panel-api/v1/static/:namespace/download/:name":              "下载静态资源",

	"POST /panel-api/v1/gpu/enabled-gpu":          "开启或关闭 GPU",
	"POST /panel-api/v1/gpu/install-hami":         "安装 HAMi",
	"POST /panel-api/v1/gpu/install-gpu-operator": "安装 GPU Operator",
	"POST /panel-api/v1/gpu/gpustack/worker":      "创建 GPUStack Worker",

	"* /panel-api/v1/files/webdav-agent/:pid/subagent/:subpid/agent/*path":   "访问子代理 WebDAV 文件",
	"* /panel-api/v1/files/webdav-agent/:pid/agent/*path":                    "访问 WebDAV 文件",
	"* /panel-api/v1/files/webdav-test/*path":                                "测试 WebDAV 文件访问",
	"* /panel-api/v1/namespaces/:namespace/services/:name/proxy-no/*path":    "免登录代理访问服务",
	"POST /panel-api/v1/files/compress-agent/:pid/compress":                  "压缩文件",
	"POST /panel-api/v1/files/compress-agent/:pid/extract":                   "解压文件",
	"POST /panel-api/v1/files/compress-agent/:pid/subagent/:subpid/compress": "压缩子代理文件",
	"POST /panel-api/v1/files/compress-agent/:pid/subagent/:subpid/extract":  "解压子代理文件",
	"POST /panel-api/v1/files/permission-agent/:pid/chmod":                   "修改文件权限",
	"POST /panel-api/v1/files/permission-agent/:pid/chown":                   "修改文件所有者",
	"POST /panel-api/v1/files/permission-agent/:pid/subagent/:subpid/chmod":  "修改子代理文件权限",
	"POST /panel-api/v1/files/permission-agent/:pid/subagent/:subpid/chown":  "修改子代理文件所有者",
	"POST /panel-api/v1/files/upload-chunk":                                  "上传文件分片",
	"POST /panel-api/v1/files/merge-chunks":                                  "合并文件分片",
	"POST /panel-api/v1/files/mvtopod":                                       "移动文件到 Pod",
	"* /panel-api/v1/s3bucket":                                               "上传 S3 文件",
	"* s3bucket":                                                             "上传 S3 文件",
	"* /panel-api/v1/microapp/:name/proxy/*path":                             "代理访问微应用",
	"POST /panel-api/v1/container/image/export-push":                         "导出并推送容器镜像",

	"PUT /panel-api/v1/zpk/install":             "安装或更新 ZPK 应用",
	"* /panel-api/v1/zpk/build-image-success":   "处理镜像构建成功回调",
	"POST /panel-api/v1/zpk/trandition/install": "安装传统应用",
	"POST /panel-api/v1/zpk/helm/memory":        "生成 Helm 内存配置",
	"POST /panel-api/v1/zpk/buildimage/job":     "创建镜像构建 Job",
	"POST /panel-api/v1/zpk/buildimage/cronjob": "创建镜像构建定时 Job",
}

func lookupRouteDescription(method string, route string) string {
	if route == "" {
		return ""
	}
	key := strings.ToUpper(method) + " " + route
	if description := routeDescriptions[key]; description != "" {
		return description
	}
	return routeDescriptions["* "+route]
}

func methodDescription(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "查询"
	case "POST":
		return "创建或提交"
	case "PUT":
		return "更新"
	case "PATCH", "PROPPATCH":
		return "部分更新"
	case "DELETE":
		return "删除"
	case "MKCOL":
		return "创建目录"
	case "COPY":
		return "复制"
	case "MOVE":
		return "移动"
	case "LOCK":
		return "锁定"
	case "UNLOCK":
		return "解锁"
	case "LINK":
		return "创建链接"
	case "UNLINK":
		return "删除链接"
	default:
		return "操作"
	}
}
