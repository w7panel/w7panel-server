package middleware

import (
	"bytes"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	microappsettingv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	"k8s.io/apimachinery/pkg/types"
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
		if siteName, err := loadMicroAppSiteName(microappConfig); err != nil {
			slog.Warn("load microapp site name failed", "err", err)
		} else if siteName != "" {
			htmlContent = replaceHTMLTitle(htmlContent, siteName)
		}
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

func loadMicroAppSiteName(microappConfig map[string]any) (string, error) {
	name, _ := microappConfig["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}

	sdk := k8s.NewK8sClient().Sdk
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return "", err
	}

	setting := &microappsettingv1alpha1.MicroAppSetting{}
	err = sigClient.Get(sdk.Ctx, types.NamespacedName{
		Name:      name,
		Namespace: sdk.GetNamespace(),
	}, setting)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(setting.Spec.General.SiteName), nil
}

func replaceHTMLTitle(htmlContent []byte, title string) []byte {
	title = strings.TrimSpace(title)
	if title == "" {
		return htmlContent
	}

	re := regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title>`)
	replaced := re.ReplaceAll(htmlContent, []byte("<title>"+html.EscapeString(title)+"</title>"))
	if !bytes.Equal(replaced, htmlContent) {
		return replaced
	}

	return htmlContent
}

func slicesInsert(src []byte, index int, insert []byte) []byte {
	result := make([]byte, 0, len(src)+len(insert))
	result = append(result, src[:index]...)
	result = append(result, insert...)
	result = append(result, src[index:]...)
	return result
}
