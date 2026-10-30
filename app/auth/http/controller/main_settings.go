package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/site"
	"github.com/w7panel/w7panel/common/service/panelauth"
	microappsetting "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// MainSettings exposes only display data to a validated CKM session on the host.
type MainSettings struct{}

func (MainSettings) Get(ctx *gin.Context) {
	value, _ := ctx.Get("panel_principal")
	principal, ok := value.(panelauth.Principal)
	if !ok || principal.TokenUse != panelauth.TokenUseCKMPanel || helper.IsChildAgent() {
		ctx.AbortWithStatus(http.StatusForbidden)
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	setting, err := site.GetGlobalMicroAppSetting(ctx.Request.Context(), sdk)
	if err != nil && !apierrors.IsNotFound(err) {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "读取主集群站点设置失败"})
		return
	}
	configMaps := map[string]*corev1.ConfigMap{}
	if setting != nil {
		for _, ref := range []microappsetting.ConfigMapRef{
			setting.Spec.Login.ProtocolConfig.UserAgreement,
			setting.Spec.Login.ProtocolConfig.PrivacyPolicy,
			setting.Spec.General.SiteLogo,
		} {
			if ref.Name == "" || ref.Key == "" || configMaps[ref.Name] != nil {
				continue
			}
			cm, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(ctx.Request.Context(), ref.Name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "读取主集群站点内容失败"})
				return
			}
			if cm.Labels["w7.cc/noauth"] != "true" {
				continue
			}
			configMaps[ref.Name] = cm
		}
	}
	domain, err := sdk.GetConfigCRD(ctx.Request.Context(), k8s.DomainParseConfigGVR, k8s.DomainParseConfigName)
	if err != nil && !apierrors.IsNotFound(err) {
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "读取主集群域名解析配置失败"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"code": 200, "data": mainSettingsDisplay(setting, configMaps, domain)})
}

func mainSettingsDisplay(setting *microappsetting.MicroAppSetting, sources map[string]*corev1.ConfigMap, domain *unstructured.Unstructured) gin.H {
	result := gin.H{"siteSetting": nil, "configMaps": map[string]gin.H{}, "domainParse": nil}
	if setting != nil {
		login := setting.Spec.Login
		general := setting.Spec.General
		contacts := make([]gin.H, 0, len(general.ContactConfigs))
		for _, contact := range general.ContactConfigs {
			contacts = append(contacts, gin.H{"type": contact.Type, "name": contact.Name, "link": contact.Link, "text": contact.Text, "qrcode": contact.Qrcode})
		}
		result["siteSetting"] = gin.H{"spec": gin.H{
			"login": gin.H{
				"loginMode": login.LoginMode, "registrationEnabled": login.RegistrationEnabled, "indexPage": login.IndexPage,
				"protocolConfig": gin.H{
					"userAgreement": displayConfigMapRef(login.ProtocolConfig.UserAgreement),
					"privacyPolicy": displayConfigMapRef(login.ProtocolConfig.PrivacyPolicy),
				},
			},
			"general": gin.H{
				"siteName": general.SiteName, "siteDescription": general.SiteDescription,
				"siteLogo": displayConfigMapRef(general.SiteLogo),
				"filing": gin.H{
					"icp":                              general.Filing.ICP,
					"publicSecurityNetworkFiling":      general.Filing.PublicSecurityNetworkFiling,
					"electronicBusinessLicense":        general.Filing.ElectronicBusinessLicense,
					"valueAddedTelecomBusinessLicense": general.Filing.ValueAddedTelecomBusinessLicense,
				},
				"contactConfigs": contacts,
			},
		}}
		selected := result["configMaps"].(map[string]gin.H)
		for _, ref := range []microappsetting.ConfigMapRef{login.ProtocolConfig.UserAgreement, login.ProtocolConfig.PrivacyPolicy, general.SiteLogo} {
			if ref.Name == "" || ref.Key == "" {
				continue
			}
			cm := sources[ref.Name]
			if cm == nil || cm.Labels["w7.cc/noauth"] != "true" {
				continue
			}
			if selected[ref.Name] == nil {
				selected[ref.Name] = gin.H{"data": map[string]string{}, "binaryData": map[string][]byte{}}
			}
			if value, ok := cm.Data[ref.Key]; ok {
				selected[ref.Name]["data"].(map[string]string)[ref.Key] = value
			}
			if value, ok := cm.BinaryData[ref.Key]; ok {
				selected[ref.Name]["binaryData"].(map[string][]byte)[ref.Key] = value
			}
			if ref == general.SiteLogo {
				selected[ref.Name]["logoImageType"] = cm.Annotations["w7.cc/logo-imagetype"]
			}
		}
	}
	if domain != nil {
		spec := k8s.ParseDomainParseConfigCRDSpec(domain)
		result["domainParse"] = gin.H{"spec": gin.H{"type": spec.Type, "cname": spec.Cname, "ips": spec.IPs}}
	}
	return result
}

func displayConfigMapRef(ref microappsetting.ConfigMapRef) gin.H {
	return gin.H{"name": ref.Name, "key": ref.Key}
}
