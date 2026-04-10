package controller

import (
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/registry"
	cd "github.com/w7panel/w7panel/common/service/registry/containerd"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
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
	// type Image struct {
	// 	Name string `json:"name"`
	// 	Tag  string `json:"tag"`
	// }
	filters := []string{"dangling=false"}
	images, err := registry.ImagesList(ctx, client, filters, []string{})
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
	defer client.Close()
	type ParamsValidate struct {
		Source string `form:"source" binding:"required"`
		Target string `form:"target" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	err = registry.Tag(http, client, params.Source, params.Target)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

func (self Images) Remove(http *gin.Context) {

	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer client.Close()
	type ParamsValidate struct {
		Force  bool   `form:"force"`
		Async  bool   `form:"async"`
		Target string `form:"target" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	err = registry.ImagesRemove(http, client, []string{params.Target}, params.Force, params.Async)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

func (self Images) Label(http *gin.Context) {

	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer client.Close()
	type ParamsValidate struct {
		Name    string            `json:"name" binding:"required"`
		Labels  map[string]string `json:"labels" binding:"required"`
		Replace bool              `json:"replace" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	err = registry.ImagesLabel(http, client, params.Name, params.Labels, params.Replace)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

// 导入镜像
func (self Images) Import(http *gin.Context) {
	client, err := cd.CreateClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer client.Close()
	type ParamsValidate struct {
		Name   string `form:"name" binding:"required"`
		Path   string `form:"path" binding:"required"`
		Pinned bool   `form:"pinned"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	importPath := params.Path
	if helper.IsAgent() || helper.IsK3kVirtual() {
		// importPath = filepath.Join("/host", importPath)
		baseDir := facade.GetConfig().GetString("s3.base_dir")
		importPath = filepath.Join(baseDir, importPath)
	}

	imgName, err := registry.ImagesImportFromFile(http, client, params.Name, importPath)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	self.JsonResponseWithoutError(http, gin.H{"name": imgName})

}
