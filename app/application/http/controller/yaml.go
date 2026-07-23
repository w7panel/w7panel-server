package controller

import (
	"io"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/kompose"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Yaml struct {
	controller.Abstract
}

func (self Yaml) ApplyYamlOld(http *gin.Context) {
	r := http.Request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer r.Body.Close()
	namespace := r.URL.Query().Get("namespace")
	token := http.MustGet("k8s_token").(string)
	client, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	// client := k8s.NewK8sClient()
	if namespace == "" {
		namespace = client.GetNamespace()
	}
	err = client.ApplyBytes(body, *k8s.NewApplyOptions(namespace))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
}

func (self Yaml) ConvertDockerComposeOld(http *gin.Context) {

	// type ParamsValidate struct {
	// 	Namespace string `form:"namespace"`
	// }
	// params := ParamsValidate{}
	// if !self.Validate(http, &params) {
	// 	return
	// }
	r := http.Request
	body, err := io.ReadAll(http.Request.Body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer r.Body.Close()

	result, err := kompose.ConvertToK8sYaml(body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	http.JSON(200, result)

}

func (self Yaml) Rollback(http *gin.Context) {
	type ParamsValidate struct {
		Namespace  string `form:"namespace" binding:"required"`
		Name       string `form:"name" binding:"required"`
		Kind       string `form:"kind" binding:"required"`
		ApiVersion string `form:"apiVersion" binding:"required"`
		toRevision int64  `form:"toRevision" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	client, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	mapping, err := client.GetRestMapping(params.ApiVersion, params.Kind)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	rawObject, err := client.GetK8sRawObject(params.Name, params.ApiVersion, params.Kind, params.Namespace)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	result, err := client.RollBack(rawObject, mapping, params.toRevision)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	http.JSON(200, gin.H{"message": result})

}
