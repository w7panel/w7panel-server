package higress

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/higress/client/pkg/apis/extensions/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var bfck = false

const (
	whiteDomainPluginName       = "w7panel-pluginwhitedomain"
	legacyWhiteDomainPluginName = "w7-white-domain"
	higressSystemNamespace      = "higress-system"
)

// 是否需要备案检测
func NeedCheckBeian() bool {
	return false //子集群pod 获取不到备案信息
	// return bfck
}

func LoadBkConfig() (*v1alpha1.WasmPlugin, error) {
	sdk := k8s.NewK8sClient()
	k8sClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}

	result := &v1alpha1.WasmPlugin{}
	err = k8sClient.Get(sdk.Ctx, types.NamespacedName{Name: whiteDomainPluginName, Namespace: higressSystemNamespace}, result)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		result = &v1alpha1.WasmPlugin{}
		err = k8sClient.Get(sdk.Ctx, types.NamespacedName{Name: legacyWhiteDomainPluginName, Namespace: higressSystemNamespace}, result)
		if err != nil {
			return nil, err
		}
	}
	WebhookWasmPlugin(result)
	return result, nil
}

func isWhiteDomainPlugin(wasmPlugin *v1alpha1.WasmPlugin) bool {
	return wasmPlugin != nil && wasmPlugin.Namespace == higressSystemNamespace &&
		(wasmPlugin.Name == whiteDomainPluginName || wasmPlugin.Name == legacyWhiteDomainPluginName)
}

func CheckHost(host string) error {
	plugin, err := LoadBkConfig()
	if err != nil {
		slog.Error("load bk config error", "error", err)
		return err
	}
	config := plugin.Spec.DefaultConfig
	val, ok := config.Fields["white_domains"]
	if !ok {
		return nil
	}
	vals := val.GetListValue().Values
	allowDomains := []string{}
	for _, v := range vals {
		structVal := v.GetStructValue()
		if structVal.Fields["enable"].GetBoolValue() {
			allowDomains = append(allowDomains, structVal.Fields["domain"].GetStringValue())
		}
	}
	//allowDomains w7.com w7.net 这种域名 fix containsAny
	// allowDomains = append(allowDomains, strings.Split(host, ".")...)

	for _, allowDomain := range allowDomains {
		if strings.Contains(host, allowDomain) {
			return nil
		}
	}
	return errors.New("host not in white list")
}

func WebhookWasmPlugin(wasmPlugin *v1alpha1.WasmPlugin) error {
	// wasmPlugin.Spec.DefaultConfig
	if isWhiteDomainPlugin(wasmPlugin) {
		bfck = !wasmPlugin.Spec.DefaultConfigDisable
	}
	return nil

}
