package order

import "github.com/w7panel/w7panel/common/service/console"

func (k *K3kOrderApi) FindK3kOrder(userName string, orderSn string) (*console.K3kOrder, error) {
	return k.consoleSdkClient.FindK3kOrder(userName, orderSn)
}

func (k *K3kOrderApi) ReturnOrderFinish(userName string, orderSn string) (*console.LastReturnOrder, error) {
	return k.consoleSdkClient.ReturnOrderFinish(userName, orderSn)
}
