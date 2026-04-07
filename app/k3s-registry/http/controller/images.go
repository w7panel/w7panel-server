package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/registry"
	cd "github.com/w7panel/w7panel/common/service/registry/containerd"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Images struct {
	controller.Abstract
}

// Run 提交容器为新镜像
func (self Images) List(ctx *gin.Context) {

	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	defer client.Close()
	type Image struct {
		Name string `json:"name"`
		Tag  string `json:"tag"`
	}

	images, err := registry.ImagesList(ctx, client, []string{}, []string{})
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	self.JsonResponseWithoutError(ctx, images)

}

// 修改镜像name
func (self Images) Tag(http *gin.Context) {

	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	type ParamsValidate struct {
		Source string `form:"source" binding:"required"`
		Target string `form:"target" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	defer client.Close()
	type Image struct {
		Name string `json:"name"`
		Tag  string `json:"tag"`
	}

	err = registry.Tag(http, client, params.Source, params.Target)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

func (self Images) Delete(http *gin.Context) {

	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	type ParamsValidate struct {
		Force  bool     `json:"force"`
		Async  bool     `json:"async"`
		Target []string `form:"target" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	defer client.Close()

	err = registry.ImagesRemove(http, client, params.Target, params.Force, params.Async)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}
