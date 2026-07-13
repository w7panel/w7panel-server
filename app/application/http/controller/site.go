package controller

import (
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Site struct {
	controller.Abstract
}

func (self Site) Beian(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	obj, err := sdk.GetConfigCRD(http.Request.Context(), k8s.FilingConfigGVR, k8s.FilingConfigName)
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}

	self.JsonResponseWithoutError(http, filingConfigResponse(k8s.ParseFilingConfigCRDSpec(obj)))
}

func (self Site) Beian2(http *gin.Context) {
	sdk := k8s.NewK8sClientInner()
	obj, err := sdk.GetConfigCRD(http.Request.Context(), k8s.FilingConfigGVR, k8s.FilingConfigName)
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}

	self.JsonResponseWithoutError(http, filingConfigResponse(k8s.ParseFilingConfigCRDSpec(obj)))
}

func filingConfigResponse(spec k8s.FilingConfigCRDSpec) gin.H {
	response := gin.H{}
	if spec.IcpNumber != "" {
		response["icpnumber"] = spec.IcpNumber
	}
	if spec.Number != "" {
		response["number"] = spec.Number
	}
	if spec.Location != "" {
		response["location"] = spec.Location
	}
	if spec.License != "" {
		response["license"] = spec.License
	}
	if spec.Tbol != "" {
		response["tbol"] = spec.Tbol
	}
	return response
}

func (self Site) K3kConfig(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	dataMap, err := sdk.GetConfigCRDData(http, k8s.K3kConfigGVR, k8s.K3kConfigName)
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}

	response := gin.H{}
	if data, ok := dataMap["indexpage"]; ok {
		response["indexpage"] = data
	}

	self.JsonResponseWithoutError(http, response)
}

func (self Site) InitUser(http *gin.Context) {
	releaseName := facade.Config.GetString("app.helm_release_name")
	sdk := k8s.NewK8sClient()

	response := gin.H{
		"canInitUser":          "false",
		"allowConsoleRegister": "false",
		"captchaEnabled":       "false",
	}

	_, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(http, releaseName+"-init-user", metav1.GetOptions{})
	if err == nil {
		response["canInitUser"] = "true"
	}

	if facade.Config.GetBool("captcha.enabled") {
		response["captchaEnabled"] = "true"
	}

	dataMap, err := sdk.GetConfigCRDData(http, k8s.K3kConfigGVR, k8s.K3kConfigName)
	if err == nil {
		if data, ok := dataMap["allowConsoleRegister"]; ok {
			response["allowConsoleRegister"] = data
		}
	}

	self.JsonResponseWithoutError(http, response)
}

func (self Site) Lianxi(http *gin.Context) {
	sdk := k8s.NewK8sClient()

	list, err := sdk.DynamicClient().Resource(k8s.ContactConfigGVR).List(http.Request.Context(), metav1.ListOptions{})
	if err != nil {
		self.JsonResponseWithoutError(http, gin.H{"items": []gin.H{}})
		return
	}
	self.JsonResponseWithoutError(http, list)

}

// TODO 子集群的configmap获取不到 未登录情况下无法获取到子集群的configmap
func (self Site) NoAuthConfigMap(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	name := http.Param("name")

	configMap, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(http, name, metav1.GetOptions{})
	if err != nil {
		self.JsonResponseWithoutError(http, corev1.ConfigMap{})
		return
	}
	if configMap.Labels == nil {
		configMap.Labels = make(map[string]string)
	}
	if configMap.Labels["w7.cc/noauth"] != "true" {
		self.JsonResponseWithoutError(http, corev1.ConfigMap{})
		return
	}
	self.JsonResponseWithoutError(http, configMap)

}

func (self Site) Registries(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	// name := http.Param("name")

	configMap, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(http, "registries", metav1.GetOptions{})
	if err != nil {
		self.JsonResponseWithoutError(http, corev1.ConfigMap{})
		return
	}
	if configMap.Data["default.cnf"] != "" {
		self.JsonResponseWithoutError(http, configMap.Data["default.cnf"])
	}

	self.JsonResponseWithoutError(http, "")

}
