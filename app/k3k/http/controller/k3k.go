package controller

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type K3k struct {
	controller.Abstract
}

func (self K3k) Info(http *gin.Context) {
	user, err := k3k.TokenToK3kUser(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if user == nil {
		self.JsonResponseWithoutError(http, map[string]string{})
		return
	}
	self.JsonResponseWithoutError(http, user.ToArray())
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
