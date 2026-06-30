package middleware

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	microappsettingv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"

	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Html struct {
	middleware.Abstract
}

const globalMicroAppSettingName = "default"

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
		if siteConfig, err := loadMicroAppSiteConfig(ctx.Request.Context(), microappConfig); err != nil {
			slog.Warn("load microapp site config failed", "err", err)
		} else if len(siteConfig) > 0 {
			microappConfig["site"] = siteConfig
			if siteName, _ := siteConfig["siteName"].(string); siteName != "" {
				htmlContent = replaceHTMLTitle(htmlContent, siteName)
			}
		}
		htmlContent = injectMicroAppScript(htmlContent, microappConfig)
	}

	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("Expires", "0")
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
		if field == "do" {
			if !strings.HasPrefix(value, "#") {
				value = "#" + value
				config["do"] = value
				continue
			}
		}

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

func loadMicroAppSiteConfig(ctx context.Context, microappConfig map[string]any) (map[string]any, error) {
	name, _ := microappConfig["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}

	sdk := k8s.NewK8sClient().Sdk
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}

	setting, err := getMicroAppSetting(ctx, sigClient, sdk.GetNamespace(), name)
	if err != nil {
		if name == globalMicroAppSettingName {
			return nil, err
		}
		globalSetting, globalErr := getMicroAppSetting(ctx, sigClient, sdk.GetNamespace(), globalMicroAppSettingName)
		if globalErr != nil {
			return nil, err
		}
		return buildMicroAppSiteConfig(ctx, sdk, nil, globalSetting), nil
	}

	if name == globalMicroAppSettingName {
		return buildMicroAppSiteConfig(ctx, sdk, setting, nil), nil
	}

	globalSetting, err := getMicroAppSetting(ctx, sigClient, sdk.GetNamespace(), globalMicroAppSettingName)
	if err != nil {
		return buildMicroAppSiteConfig(ctx, sdk, setting, nil), nil
	}
	return buildMicroAppSiteConfig(ctx, sdk, setting, globalSetting), nil
}

func getMicroAppSetting(ctx context.Context, client sigclient.Client, namespace string, name string) (*microappsettingv1alpha1.MicroAppSetting, error) {
	setting := &microappsettingv1alpha1.MicroAppSetting{}
	err := client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, setting)
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func buildMicroAppSiteConfig(ctx context.Context, sdk *k8s.Sdk, setting, fallback *microappsettingv1alpha1.MicroAppSetting) map[string]any {
	general := microappGeneral(setting)
	fallbackGeneral := microappGeneral(fallback)
	login := microappLogin(setting)
	fallbackLogin := microappLogin(fallback)
	siteLogo := general.SiteLogo
	if siteLogo.Name == "" {
		siteLogo = fallbackGeneral.SiteLogo
	}

	result := map[string]any{
		"siteName":        firstNonEmpty(general.SiteName, fallbackGeneral.SiteName),
		"siteDescription": firstNonEmpty(general.SiteDescription, fallbackGeneral.SiteDescription),
		"filing":          microAppFilingConfig(general.Filing, fallbackGeneral.Filing),
		"login": map[string]any{
			"loginMode":           firstNonEmpty(login.LoginMode, fallbackLogin.LoginMode),
			"registrationEnabled": microAppRegistrationEnabled(setting, fallback),
			"indexPage":           firstNonEmpty(login.IndexPage, fallbackLogin.IndexPage),
		},
	}
	if logo := loadMicroAppLogo(ctx, sdk, siteLogo); logo != "" {
		result["logo"] = logo
	}
	return result
}

func microappGeneral(setting *microappsettingv1alpha1.MicroAppSetting) microappsettingv1alpha1.GeneralSettings {
	if setting == nil {
		return microappsettingv1alpha1.GeneralSettings{}
	}
	return setting.Spec.General
}

func microappLogin(setting *microappsettingv1alpha1.MicroAppSetting) microappsettingv1alpha1.LoginSettings {
	if setting == nil {
		return microappsettingv1alpha1.LoginSettings{}
	}
	return setting.Spec.Login
}

func microAppRegistrationEnabled(setting, fallback *microappsettingv1alpha1.MicroAppSetting) bool {
	if setting != nil {
		return setting.Spec.Login.RegistrationEnabled
	}
	if fallback != nil {
		return fallback.Spec.Login.RegistrationEnabled
	}
	return false
}

func microAppFilingConfig(filing, fallback microappsettingv1alpha1.FilingSettings) map[string]string {
	icp := firstNonEmpty(filing.ICP, fallback.ICP)
	publicSecurity := firstNonEmpty(filing.PublicSecurityNetworkFiling, fallback.PublicSecurityNetworkFiling)
	return map[string]string{
		"icp":                              icp,
		"icpnumber":                        icp,
		"publicSecurityNetworkFiling":      publicSecurity,
		"number":                           publicSecurity,
		"location":                         publicSecurity,
		"electronicBusinessLicense":        firstNonEmpty(filing.ElectronicBusinessLicense, fallback.ElectronicBusinessLicense),
		"license":                          firstNonEmpty(filing.ElectronicBusinessLicense, fallback.ElectronicBusinessLicense),
		"valueAddedTelecomBusinessLicense": firstNonEmpty(filing.ValueAddedTelecomBusinessLicense, fallback.ValueAddedTelecomBusinessLicense),
		"tbol":                             firstNonEmpty(filing.ValueAddedTelecomBusinessLicense, fallback.ValueAddedTelecomBusinessLicense),
	}
}

func loadMicroAppLogo(ctx context.Context, sdk *k8s.Sdk, ref microappsettingv1alpha1.ConfigMapRef) string {
	if ref.Name == "" || ref.Key == "" {
		return ""
	}
	configMap, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(ctx, ref.Name, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	if data := configMap.Data[ref.Key]; data != "" {
		if strings.HasPrefix(data, "data:") || strings.HasPrefix(data, "http://") || strings.HasPrefix(data, "https://") || strings.HasPrefix(data, "/") {
			return data
		}
		return "data:" + mimeType(ref.Key) + ";base64," + data
	}
	if data := configMap.BinaryData[ref.Key]; len(data) > 0 {
		prefix := configMap.Annotations["w7.cc/logo-imagetype"]
		if prefix == "" {
			prefix = "data:" + mimeType(ref.Key) + ";base64,"
		}
		return prefix + base64.StdEncoding.EncodeToString(data)
	}
	return ""
}

func mimeType(key string) string {
	if value := mime.TypeByExtension(filepath.Ext(key)); value != "" {
		return value
	}
	return "image/png"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
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
