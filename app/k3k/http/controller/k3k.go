package controller

import (
	"encoding/json"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type K3k struct {
	controller.Abstract
}

func (self K3k) Info(http *gin.Context) {
	if username := http.GetString("username"); username != "" && http.GetString("ckm_name") == "" {
		if sdk := k8s.NewK8sClient().Sdk; sdk != nil {
			if u, err := userservice.Get(http.Request.Context(), sdk, username); err == nil {
				self.JsonResponseWithoutError(http, k3ktypes.NewK3kUser(u.ToTyped()).ToArray())
				return
			}
		}
	}
	user, err := k3k.TokenToK3kUser(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if user == nil {
		self.JsonResponseWithoutError(http, map[string]string{})
		return
	}
	result := user.ToArray()
	if http.GetString("ckm_name") != "" {
		menus := make([]string, 0)
		for _, menu := range k3ktypes.K3K_MENU_FOUNDER_RULES {
			if menu != "zpk" && menu != "cluster/nodes" && menu != "cluster/nodes-image-list" {
				menus = append(menus, menu)
			}
		}
		encoded, _ := json.Marshal(menus)
		result["w7.cc/menu"] = string(encoded)
		result["w7.cc/role"] = "normal"
		result["w7.cc/user-mode"] = "cluster"
		result["w7.cc/username"] = http.GetString("username")
	}
	self.JsonResponseWithoutError(http, result)
}

func (self K3k) SyncIngress(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncIngress(&params)
	if err != nil {
		slog.Error("同步ingress失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncConfigmap(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncConfigmap(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncMcpBridge(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncMcpBridge(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncSecret(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	slog.Error("同步secret")
	err := k3k.SyncSecret(&params)
	if err != nil {
		slog.Error("同步失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncDownStatic(http *gin.Context) {
	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	slog.Error("同步down-static")
	appgroup.DownStaticGo(params.VirtualNamespace, params.VirtualName, "")
	self.JsonSuccessResponse(http)
	return
}

func (self K3k) SyncMicroApp(http *gin.Context) {
	// Microapp data is now served independently; retain this callback as a
	// successful no-op for existing CKM installations.
	self.JsonSuccessResponse(http)
}

func (self K3k) SyncSite(http *gin.Context) {

	params := k3k.K3kSync{}
	if !self.Validate(http, &params) {
		return
	}
	err := k3k.SyncSite(&params)
	if err != nil {
		slog.Error("同步site失败", "error", err)
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
	return
}
