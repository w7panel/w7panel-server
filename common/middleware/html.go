package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

/*
*
提示词:
common/middleware/html.go 根据header 中是否有microapp_变量名 必须如microapp_name microapp_do  给index.html注入

	js对象
	w7_microapp: {name: xxx, do:xxxx, leftmenu: true,breadcrumb: false ,needlogin:false}
*/
type Html struct {
	middleware.Abstract
}

func (self Html) Process(ctx *gin.Context) {
	path := ctx.Request.URL.Path

	// API 路由不处理
	if strings.HasPrefix(path, "/panel-api/") ||
		strings.HasPrefix(path, "/k8s-proxy/") ||
		strings.HasPrefix(path, "/k8s/") ||
		strings.HasPrefix(path, "/api/") {
		ctx.Status(404)
		ctx.Writer.Write([]byte(`{"code":404,"msg":"Not Found"}`))
		ctx.Abort()
		return
	}

	// 非 API 路由返回 index.html
	staticPath := facade.Config.GetString("app.static_path")
	htmlContent, err := os.ReadFile(staticPath + "/index.html")
	if err != nil {
		ctx.Status(500)
		return
	}

	if microappConfig, ok := buildMicroAppConfig(ctx); ok {
		htmlContent = injectMicroAppScript(htmlContent, microappConfig)
	}

	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Status(200)
	io.Copy(ctx.Writer, bytes.NewReader(htmlContent))
	ctx.Abort()
}

func buildMicroAppConfig(ctx *gin.Context) (map[string]any, bool) {
	config := map[string]any{
		"name":       "",
		"do":         "",
		"leftmenu":   false,
		"breadcrumb": false,
		"needlogin":  false,
	}

	found := false
	for key, values := range ctx.Request.Header {
		headerKey := strings.ToLower(key)
		if !strings.HasPrefix(headerKey, "microapp_") || len(values) == 0 {
			continue
		}

		field := strings.TrimPrefix(headerKey, "microapp_")
		if field == "" {
			continue
		}

		found = true
		value := values[0]

		switch field {
		case "leftmenu", "breadcrumb", "needlogin":
			config[field] = parseMicroAppBool(value)
		default:
			config[field] = value
		}
	}

	return config, found
}

func parseMicroAppBool(value string) bool {
	parsed, err := strconv.ParseBool(value)
	if err == nil {
		return parsed
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "yes", "on":
		return true
	default:
		return false
	}
}

func injectMicroAppScript(htmlContent []byte, microappConfig map[string]any) []byte {
	configJSON, err := json.Marshal(microappConfig)
	if err != nil {
		return htmlContent
	}

	script := []byte("<script>window.w7_microapp = " + string(configJSON) + ";</script>")
	lowerHTML := strings.ToLower(string(htmlContent))

	if index := strings.Index(lowerHTML, "</head>"); index >= 0 {
		return slicesInsert(htmlContent, index, script)
	}

	if index := strings.Index(lowerHTML, "</body>"); index >= 0 {
		return slicesInsert(htmlContent, index, script)
	}

	return append(htmlContent, script...)
}

func slicesInsert(src []byte, index int, insert []byte) []byte {
	result := make([]byte, 0, len(src)+len(insert))
	result = append(result, src[:index]...)
	result = append(result, insert...)
	result = append(result, src[index:]...)
	return result
}
