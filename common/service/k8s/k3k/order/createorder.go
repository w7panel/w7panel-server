package order

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
)

func createOrder(user *types.K3kUser, mainConfig *config.W7Config, currentConfig *config.W7Config, productId int32, params map[string]string, client *console.SdkClient) (order *console.PayResult, err error) {
	values := url.Values{}
	values.Set("productId", strconv.Itoa(int(productId)))
	values.Set("clusterId", mainConfig.ClusterId)
	values.Set("k3kName", user.Name)
	values.Set("appid", client.License.AppId)
	for k, v := range params {
		values.Add(k, v)
	}
	// return client.CreatePanelOrder(values) //sdk 导致获取的用户id 是appid 站点的bbsuid
	apiClient := console.NewConsoleCdClient(currentConfig.ThirdpartyCDToken)
	return apiClient.CreatePanelOrder(values)

}

func (k *K3kOrderApi) createOrder(baseConfigName string, user *types.K3kUser, params map[string]string, buyMode string) (*console.PayResult, error) {
	license := console.GetCurrentLicense()
	if license == nil {
		return nil, fmt.Errorf("免费版不支持购买")
	}
	baseConfigName = license.FounderSaName

	w7respo := k.w7respo
	w7config, err := w7respo.Get(baseConfigName)
	if err != nil {
		return nil, err
	}
	currentConfig, err := w7respo.Get(user.Name)
	if err != nil {
		return nil, err
	}
	// sdkClient, err := console.NewSdkClient(license)
	// if err != nil {
	// 	return nil, err
	// }
	product, err := k.consoleSdkClient.PrepareProduct2()
	// product, err := PrepareProduct(w7config)
	if err != nil {
		return nil, err
	}
	payResult, err := createOrder(user, w7config, currentConfig, product.ProductId, params, k.consoleSdkClient)
	if err != nil {
		return nil, err
	}
	// _, err = controllerutil.CreateOrPatch(k.sdk.Ctx, k.client, user.ServiceAccount, func() error {
	// 	if buyMode == BASE_BUY {
	// 		user.SetBaseOrder(payResult.OrderSn)
	// 	}
	// 	if buyMode == RENEW_BUY {
	// 		user.SetRenewOrder(payResult.OrderSn)
	// 	}
	// 	if buyMode == EXPAND_BUY {
	// 		user.SetExpandOrder(payResult.OrderSn)
	// 	}
	// 	return nil
	// })
	// if err != nil {
	// 	return nil, err
	// }
	// if !payResult.NeedPay {
	// 	time.AfterFunc(time.Second*2, func() {
	// 		k.NotifyOrder(user, payResult.OrderSn) //0延迟5秒，防止订单还未创建完成 k3kuser 可能还没创建完成，延迟5秒再通知
	// 	})
	// }

	return payResult, nil
}

func (k *K3kOrderApi) CheckCanBuyCvm(cvmName string) error {
	return nil
}
