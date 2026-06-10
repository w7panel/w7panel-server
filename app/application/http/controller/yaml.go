package controller

import (
	"fmt"
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

func (self Yaml) ApplyYaml(http *gin.Context) {
	type ParamsValidate struct {
		Yaml      []byte `form:"yaml" validate:"required"`
		Namespace string `form:"namespace" `
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	namespace := params.Namespace
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
	err = client.ApplyBytes(params.Yaml, *k8s.NewApplyOptions(namespace))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)
}

func (self Yaml) ConvertDockerCompose(http *gin.Context) {

	type ParamsValidate struct {
		Yaml []byte `form:"yaml"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	body := params.Yaml

	result, err := kompose.ConvertToK8sYaml(body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	http.JSON(200, result)

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

func (self Yaml) ApplyDockerCompose(http *gin.Context) {

	type ParamsValidate struct {
		Namespace string `form:"namespace"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	http.BindYAML(params)
	r := http.Request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	defer r.Body.Close()
	client, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	namespace := params.Namespace
	if namespace == "" {
		namespace = client.GetNamespace()
	}

	yamlMap, err := kompose.ConvertToK8sYaml(body)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	//事务问题
	for _, content := range yamlMap {
		_, err = client.ApplyYaml([]byte(content), *k8s.NewApplyOptions(namespace))
		if err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
	}

	self.JsonSuccessResponse(http)
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

func (self Yaml) OpenApi(http *gin.Context) {
	type ParamsValidate struct {
		Kind       string `form:"kind" binding:"required"`
		ApiVersion string `form:"apiVersion" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	client := k8s.NewK8sClient()

	openapiv3, err := client.OpenAPIV3Client()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	paths, err := openapiv3.Paths()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	gv, ok := paths[params.ApiVersion]
	if !ok {
		self.JsonResponseWithServerError(http, fmt.Errorf("apiVersion %s not found", params.ApiVersion))
		return
	}
	content, err := gv.Schema("application/json")
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	http.String(200, string(content))

}

// 获取k8s 模型的schema
func (self Yaml) YamlSchema(http *gin.Context) {
	type ParamsValidate struct {
		Kind    string `form:"kind" binding:"required"`
		Version string `form:"version" binding:"required"`
		Group   string `form:"Group" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	client := k8s.NewK8sClient()
	oa, _ := client.OpenAPISchema()
	http.String(200, oa.Definitions.String())
	// factory := cmdutil.NewFactory(client)
	// openapi, err := factory.OpenAPISchema()
	// if err != nil {
	// 	self.JsonResponseWithServerError(http, err)
	// 	return
	// }
	// gvk := schema.GroupVersionKind{
	// 	Group:   params.Group,
	// 	Version: params.Version,
	// 	Kind:    params.Kind,
	// }
	// schema := openapi.LookupResource(gvk)
	// if schema == nil {
	// 	self.JsonResponseWithServerError(http, fmt.Errorf("schema %s not found", params.Kind))
	// 	return
	// }
	// http.JSON(200, schema)

}

// func extractSchema(openAPISchema *openapi_v2.Document, schemaName string) *spec.Schema {
// 	definitions := openAPISchema.Paths.Definitions
// 	if schema, ok := definitions[schemaName]; ok {
// 		return &schema
// 	}
// 	return nil
// }
