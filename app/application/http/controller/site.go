package controller

import (
	"github.com/gin-gonic/gin"
	microappsettingv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/site"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Site struct {
	controller.Abstract
}

const globalMicroAppSettingName = "default"

func (self Site) Beian(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	if setting, err := getGlobalMicroAppSetting(http, sdk.Sdk); err == nil && hasMicroAppFiling(setting.Spec.General.Filing) {
		self.JsonResponseWithoutError(http, microAppFilingResponse(setting.Spec.General.Filing))
		return
	}

	obj, err := sdk.GetConfigCRD(http.Request.Context(), k8s.FilingConfigGVR, k8s.FilingConfigName)
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}

	self.JsonResponseWithoutError(http, filingConfigResponse(k8s.ParseFilingConfigCRDSpec(obj)))
}

func (self Site) Beian2(http *gin.Context) {
	sdk := k8s.NewK8sClientInner()
	if setting, err := getGlobalMicroAppSetting(http, sdk); err == nil && hasMicroAppFiling(setting.Spec.General.Filing) {
		self.JsonResponseWithoutError(http, microAppFilingResponse(setting.Spec.General.Filing))
		return
	}

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

func microAppFilingResponse(spec microappsettingv1alpha1.FilingSettings) gin.H {
	response := gin.H{}
	if spec.ICP != "" {
		response["icpnumber"] = spec.ICP
	}
	if spec.PublicSecurityNetworkFiling != "" {
		response["number"] = spec.PublicSecurityNetworkFiling
		response["location"] = spec.PublicSecurityNetworkFiling
	}
	if spec.ElectronicBusinessLicense != "" {
		response["license"] = spec.ElectronicBusinessLicense
	}
	if spec.ValueAddedTelecomBusinessLicense != "" {
		response["tbol"] = spec.ValueAddedTelecomBusinessLicense
	}
	return response
}

func hasMicroAppFiling(spec microappsettingv1alpha1.FilingSettings) bool {
	return spec.ICP != "" ||
		spec.PublicSecurityNetworkFiling != "" ||
		spec.ElectronicBusinessLicense != "" ||
		spec.ValueAddedTelecomBusinessLicense != ""
}

func (self Site) K3kConfig(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	response := gin.H{}
	if setting, err := getGlobalMicroAppSetting(http, sdk.Sdk); err == nil && setting.Spec.Login.IndexPage != "" {
		response["indexpage"] = setting.Spec.Login.IndexPage
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

	if setting, err := getGlobalMicroAppSetting(http, sdk.Sdk); err == nil {
		response["allowConsoleRegister"] = boolString(setting.Spec.Login.RegistrationEnabled)
	}
	self.JsonResponseWithoutError(http, response)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func getGlobalMicroAppSetting(http *gin.Context, sdk *k8s.Sdk) (*microappsettingv1alpha1.MicroAppSetting, error) {
	return site.GetGlobalMicroAppSetting(http, sdk)
}

func (self Site) Lianxi(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	if setting, err := getGlobalMicroAppSetting(http, sdk.Sdk); err == nil && len(setting.Spec.General.ContactConfigs) > 0 {
		self.JsonResponseWithoutError(http, gin.H{"items": microAppContactItems(setting.Spec.General.ContactConfigs)})
		return
	}

	list, err := sdk.DynamicClient().Resource(k8s.ContactConfigGVR).List(http.Request.Context(), metav1.ListOptions{})
	if err != nil {
		self.JsonResponseWithoutError(http, gin.H{"items": []gin.H{}})
		return
	}
	self.JsonResponseWithoutError(http, list)

}

func microAppContactItems(configs []microappsettingv1alpha1.ContactConfigSettings) []gin.H {
	items := make([]gin.H, 0, len(configs))
	for index, config := range configs {
		name := config.Name
		if name == "" {
			name = "contact-us"
		}
		items = append(items, gin.H{
			"metadata": gin.H{
				"name": name,
			},
			"spec": gin.H{
				"type":     config.Type,
				"link":     config.Link,
				"text":     config.Text,
				"name":     config.Name,
				"showName": config.ShowName,
				"selicon":  config.SelIcon,
				"icon":     config.Icon,
				"qrcode":   config.Qrcode,
				"style":    config.Style,
				"index":    contactIndex(config.Index, index),
			},
		})
	}
	return items
}

func contactIndex(value int32, fallback int) int32 {
	if value > 0 {
		return value
	}
	return int32(fallback + 1)
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
		cfg := configMap.Data["default.cnf"]
		_, err := helper.YamlParse([]byte(cfg))
		if err != nil {
			self.JsonResponseWithoutError(http, "")
			return
		}
		//TO yaml string

		// cfg2, err := yaml.Marshal(result)
		// if err != nil {
		// 	self.JsonResponseWithoutError(http, "")
		// 	return
		// }
		http.String(200, string(cfg))
		return

	}

	self.JsonResponseWithoutError(http, "")

}
