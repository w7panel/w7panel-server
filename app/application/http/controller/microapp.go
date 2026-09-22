package controller

import (
	// "github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"fmt"
	stdhttp "net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s/microapp"
	"github.com/w7panel/w7panel/common/service/oidc"
	"github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type MicroApp struct {
	controller.Abstract
}

type normalMicroApp struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func microAppGroupName(item *v1alpha1.MicroApp) string {
	if item == nil {
		return ""
	}
	if groupName := item.Labels["w7.cc/group-name"]; groupName != "" {
		return groupName
	}
	return item.Name
}

func (self MicroApp) List(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	list, err := microapp.ListTop(token, http.GetString("user_mode"))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, list)

}

// 普通用户列表，不包括管理员应用
func (self MicroApp) TopNormal(http *gin.Context) {
	list, err := microapp.ListByRole("normal")
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	items, err := normalMicroApps(list)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.normalMicroAppsResponse(http, items)
}

func (self MicroApp) normalMicroAppsResponse(http *gin.Context, items []normalMicroApp) {
	if http.Query("callback") != "" {
		http.JSONP(stdhttp.StatusOK, items)
		return
	}
	self.JsonResponseWithoutError(http, items)
}

func normalMicroApps(list *v1alpha1.MicroAppList) ([]normalMicroApp, error) {
	mainPanelURL, err := mainPanelURL()
	if err != nil {
		return nil, err
	}
	items := make([]normalMicroApp, 0, len(list.Items))
	for _, item := range list.Items {
		items = append(items, normalMicroApp{Title: item.Spec.Title, URL: mainPanelURL + "/appgroup/" + item.Name + "/micro"})
	}
	return items, nil
}

func mainPanelURL() (string, error) {
	raw := strings.TrimSpace(os.Getenv("MAIN_PANEL_URL"))
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("MAIN_PANEL_URL must be an HTTPS origin")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func (self MicroApp) Info(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	microappObj, err := microapp.ListInfo(token, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithoutError(http, microappObj)
		return
	}
	configs := microappObj.Spec.ConfigV2.Props.RoleConfig
	if configs != nil {
		self.JsonResponseWithoutError(http, microappObj)
		return
	}
	for role, _ := range configs {
		config := microappObj.Spec.ConfigV2.Props.RoleConfig[role]
		props := replace.Replace(http, config.FrontendProps, role, microappObj)
		config.FrontendProps = props
	}

	self.JsonResponseWithoutError(http, microappObj)

}

func (self MicroApp) FrontProps(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	item, err := microapp.ListInfo(token, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	role := replace.GetRole()
	accessToken := ""
	server, err := oidc.GetServer()
	if err == nil && server != nil {
		accessToken, err = server.CreateDefaultAccessToken(server.ContextWithIssuer(http.Request.Context(), http.Request), replace.Name)
		if err != nil {
			accessToken = ""
		}
	}

	cloudAccessToken := ""
	if replace.GetConsoleOpenId() != "" {
		token, err := microapp.GetCloudAccessToken(replace.GetConsoleOpenId())
		if err == nil {
			cloudAccessToken = token
		}
	}

	groupName := microAppGroupName(item)
	self.JsonResponseWithoutError(http, map[string]string{
		// "url":               item.RoleServerUrl(role),
		"group":             groupName,
		"appgroup":          groupName,
		"userid":            replace.Name,
		"role":              role,
		"access_token":      accessToken,
		"openid":            replace.GetConsoleOpenId(),
		"nickname":          replace.GetNickName(),
		"cloud_uid":         replace.GetConsoleId(),
		"cloud_accesstoken": cloudAccessToken,
	})
}

func (self MicroApp) GlobalFrontProps(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)

	replace, err := microapp.NewMicroAppReplace(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	role := replace.GetRole()
	accessToken := ""
	server, err := oidc.GetServer()
	if err == nil && server != nil {
		accessToken, err = server.CreateDefaultAccessToken(server.ContextWithIssuer(http.Request.Context(), http.Request), replace.Name)
		if err != nil {
			accessToken = ""
		}
	}

	cloudAccessToken := ""
	if replace.GetConsoleOpenId() != "" {
		token, err := microapp.GetCloudAccessToken(replace.GetConsoleOpenId())
		if err == nil {
			cloudAccessToken = token
		}
	}

	self.JsonResponseWithoutError(http, map[string]string{
		// "url":               item.RoleServerUrl(role),
		"userid":            replace.Name,
		"role":              role,
		"access_token":      accessToken,
		"openid":            replace.GetConsoleOpenId(),
		"nickname":          replace.GetNickName(),
		"cloud_uid":         replace.GetConsoleId(),
		"cloud_accesstoken": cloudAccessToken,
	})
}
