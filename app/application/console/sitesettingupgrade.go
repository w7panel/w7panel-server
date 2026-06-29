package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultSiteSettingName      = "default"
	defaultSiteSettingConfigMap = "default-settings"
	legacyLogoNamespace         = "kube-system"
	legacyLogoConfigMap         = "k3k.logo.config"
)

type SiteSettingUpgrade struct {
	console2.Abstract
}

type siteSettingUpgradeOption struct {
	namespace string
	name      string
	configMap string
	overwrite bool
}

var siteSettingUpgradeOp = siteSettingUpgradeOption{}

func (c SiteSettingUpgrade) GetName() string {
	return "site-setting-upgrade"
}

func (c SiteSettingUpgrade) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&siteSettingUpgradeOp.namespace, "namespace", "", "站点设置所在命名空间，默认使用面板命名空间")
	cmd.Flags().StringVar(&siteSettingUpgradeOp.name, "name", defaultSiteSettingName, "MicroAppSetting 名称")
	cmd.Flags().StringVar(&siteSettingUpgradeOp.configMap, "configmap", defaultSiteSettingConfigMap, "站点设置内容 ConfigMap 名称")
	cmd.Flags().BoolVar(&siteSettingUpgradeOp.overwrite, "overwrite", false, "覆盖已存在的新配置字段")
}

func (c SiteSettingUpgrade) GetDescription() string {
	return "升级旧站点配置到 MicroAppSetting"
}

func (c SiteSettingUpgrade) Handle(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	sdk := k8s.NewK8sClient().Sdk
	namespace := siteSettingUpgradeOp.namespace
	if namespace == "" {
		namespace = sdk.GetNamespace()
	}

	legacy := c.loadLegacyConfig(ctx, sdk)
	if err := c.upsertSettingConfigMap(ctx, sdk, namespace, siteSettingUpgradeOp.configMap); err != nil {
		slog.Error("升级站点设置 ConfigMap 失败", "error", err)
		return
	}
	if err := c.upsertMicroAppSetting(ctx, sdk, namespace, siteSettingUpgradeOp.name, siteSettingUpgradeOp.configMap, legacy); err != nil {
		slog.Error("升级站点设置失败", "error", err)
		return
	}
	slog.Info("升级站点设置完成", "namespace", namespace, "name", siteSettingUpgradeOp.name, "configmap", siteSettingUpgradeOp.configMap)
}

type legacySiteSetting struct {
	AllowConsoleRegister string
	IndexPage            string
	Filing               k8s.FilingConfigCRDSpec
	ContactConfigs       []interface{}
}

func (c SiteSettingUpgrade) loadLegacyConfig(ctx context.Context, sdk *k8s.Sdk) legacySiteSetting {
	setting := legacySiteSetting{
		IndexPage: "login",
	}

	if data, err := sdk.GetConfigCRDData(ctx, k8s.K3kConfigGVR, k8s.K3kConfigName); err == nil {
		setting.AllowConsoleRegister = data["allowConsoleRegister"]
		if data["indexpage"] != "" {
			setting.IndexPage = data["indexpage"]
		}
	} else if !apierrors.IsNotFound(err) {
		slog.Warn("读取旧注册配置失败", "error", err)
	}

	if filing, err := sdk.GetConfigCRD(ctx, k8s.FilingConfigGVR, k8s.FilingConfigName); err == nil {
		setting.Filing = k8s.ParseFilingConfigCRDSpec(filing)
	} else if !apierrors.IsNotFound(err) {
		slog.Warn("读取旧备案配置失败", "error", err)
	}
	setting.ContactConfigs = c.loadLegacyContactConfigs(ctx, sdk)

	return setting
}

func (c SiteSettingUpgrade) loadLegacyContactConfigs(ctx context.Context, sdk *k8s.Sdk) []interface{} {
	list, err := sdk.DynamicClient().Resource(k8s.ContactConfigGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			slog.Warn("读取旧联系方式配置失败", "error", err)
		}
		return nil
	}
	contacts := make([]interface{}, 0, len(list.Items))
	for _, item := range list.Items {
		spec := k8s.ParseContactConfigCRDSpec(&item)
		contacts = append(contacts, map[string]interface{}{
			"type":     spec.Type,
			"link":     spec.Link,
			"text":     spec.Text,
			"name":     spec.Name,
			"showName": spec.ShowName,
			"selicon":  spec.SelIcon,
			"icon":     spec.Icon,
			"qrcode":   spec.Qrcode,
			"style":    spec.Style,
			"index":    int64(spec.Index),
		})
	}
	return contacts
}

func (c SiteSettingUpgrade) upsertSettingConfigMap(ctx context.Context, sdk *k8s.Sdk, namespace, name string) error {
	configMaps := sdk.ClientSet.CoreV1().ConfigMaps(namespace)
	configMap, err := configMaps.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string]string{},
		}
	} else {
		configMap = configMap.DeepCopy()
	}

	if configMap.Labels == nil {
		configMap.Labels = map[string]string{}
	}
	configMap.Labels["w7.cc/noauth"] = "true"
	if configMap.Annotations == nil {
		configMap.Annotations = map[string]string{}
	}
	if configMap.Data == nil {
		configMap.Data = map[string]string{}
	}
	if _, ok := configMap.Data["user-agreement.html"]; !ok {
		configMap.Data["user-agreement.html"] = ""
	}
	if _, ok := configMap.Data["privacy-policy.html"]; !ok {
		configMap.Data["privacy-policy.html"] = ""
	}

	c.migrateLogo(ctx, sdk, configMap)

	if configMap.ResourceVersion == "" {
		_, err = configMaps.Create(ctx, configMap, metav1.CreateOptions{})
		return err
	}
	_, err = configMaps.Update(ctx, configMap, metav1.UpdateOptions{})
	return err
}

func (c SiteSettingUpgrade) migrateLogo(ctx context.Context, sdk *k8s.Sdk, configMap *corev1.ConfigMap) {
	if !siteSettingUpgradeOp.overwrite && len(configMap.BinaryData["logo.png"]) > 0 {
		return
	}
	legacyLogo, err := sdk.ClientSet.CoreV1().ConfigMaps(legacyLogoNamespace).Get(ctx, legacyLogoConfigMap, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			slog.Warn("读取旧 LOGO 配置失败", "error", err)
		}
		return
	}
	logo := legacyLogo.BinaryData["default-cnf"]
	if len(logo) == 0 {
		return
	}
	if configMap.BinaryData == nil {
		configMap.BinaryData = map[string][]byte{}
	}
	configMap.BinaryData["logo.png"] = logo
	if prefix := legacyLogo.Annotations["imagetype"]; prefix != "" {
		configMap.Annotations["w7.cc/logo-imagetype"] = prefix
	}
}

func (c SiteSettingUpgrade) upsertMicroAppSetting(ctx context.Context, sdk *k8s.Sdk, namespace, name, configMapName string, legacy legacySiteSetting) error {
	resource := sdk.DynamicClient().Resource(k8s.MicroAppSettingGVR).Namespace(namespace)
	setting, err := resource.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		setting = &unstructured.Unstructured{}
		setting.SetAPIVersion(k8s.ConfigCRDGroup + "/" + k8s.ConfigCRDVersion)
		setting.SetKind("MicroAppSetting")
		setting.SetName(name)
		setting.SetNamespace(namespace)
	} else {
		setting = setting.DeepCopy()
	}

	overwrite := siteSettingUpgradeOp.overwrite || setting.GetResourceVersion() == ""
	c.setString(setting, overwrite, []string{"spec", "login", "loginMode"}, "password")
	c.setBoolString(setting, overwrite, []string{"spec", "login", "registrationEnabled"}, legacy.AllowConsoleRegister)
	c.setString(setting, overwrite, []string{"spec", "login", "indexPage"}, valueOrDefault(legacy.IndexPage, "login"))
	c.setString(setting, overwrite, []string{"spec", "login", "protocolConfig", "userAgreement", "name"}, configMapName)
	c.setString(setting, overwrite, []string{"spec", "login", "protocolConfig", "userAgreement", "key"}, "user-agreement.html")
	c.setString(setting, overwrite, []string{"spec", "login", "protocolConfig", "privacyPolicy", "name"}, configMapName)
	c.setString(setting, overwrite, []string{"spec", "login", "protocolConfig", "privacyPolicy", "key"}, "privacy-policy.html")
	c.setString(setting, overwrite, []string{"spec", "general", "siteLogo", "name"}, configMapName)
	c.setString(setting, overwrite, []string{"spec", "general", "siteLogo", "key"}, "logo.png")
	c.setString(setting, overwrite, []string{"spec", "general", "filing", "icp"}, legacy.Filing.IcpNumber)
	c.setString(setting, overwrite, []string{"spec", "general", "filing", "publicSecurityNetworkFiling"}, legacy.Filing.Location)
	c.setString(setting, overwrite, []string{"spec", "general", "filing", "electronicBusinessLicense"}, legacy.Filing.License)
	c.setString(setting, overwrite, []string{"spec", "general", "filing", "valueAddedTelecomBusinessLicense"}, legacy.Filing.Tbol)
	c.setSlice(setting, overwrite, []string{"spec", "general", "contactConfigs"}, legacy.ContactConfigs)

	if setting.GetResourceVersion() == "" {
		_, err = resource.Create(ctx, setting, metav1.CreateOptions{})
		return err
	}
	_, err = resource.Update(ctx, setting, metav1.UpdateOptions{})
	return err
}

func (c SiteSettingUpgrade) setString(obj *unstructured.Unstructured, overwrite bool, fields []string, value string) {
	if value == "" {
		return
	}
	current, exists, _ := unstructured.NestedString(obj.Object, fields...)
	if overwrite || !exists || current == "" {
		_ = unstructured.SetNestedField(obj.Object, value, fields...)
	}
}

func (c SiteSettingUpgrade) setBoolString(obj *unstructured.Unstructured, overwrite bool, fields []string, value string) {
	if value == "" {
		return
	}
	_, exists, _ := unstructured.NestedBool(obj.Object, fields...)
	if overwrite || !exists {
		_ = unstructured.SetNestedField(obj.Object, value == "true", fields...)
	}
}

func (c SiteSettingUpgrade) setSlice(obj *unstructured.Unstructured, overwrite bool, fields []string, value []interface{}) {
	if len(value) == 0 {
		return
	}
	current, exists, _ := unstructured.NestedSlice(obj.Object, fields...)
	if overwrite || !exists || len(current) == 0 {
		_ = unstructured.SetNestedSlice(obj.Object, value, fields...)
	}
}

func valueOrDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
