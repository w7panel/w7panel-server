package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DomainParseConfig struct {
	console2.Abstract
}

// ./runtime/main cluster:register --thirdPartyCDToken=ywA2N3ImkVo0tPOn --registerCluster=true --offlineUrl=http://118.25.145.25:9090 --apiServerUrl=https://118.25.145.25:6443

func (c DomainParseConfig) GetName() string {
	return "domain-config"
}

func (c DomainParseConfig) Configure(cmd *cobra.Command) {
	// username password register

}

func (c DomainParseConfig) GetDescription() string {
	return "fix domain config"
}

// Handle 处理域名解析配置命令，获取当前IP地址并创建 domain-parse CRD
// 如果 CRD 已存在，则不执行任何操作
func (c DomainParseConfig) Handle(cmd *cobra.Command, args []string) {
	ip, err := helper.MyIp()
	if err != nil {
		slog.Error("Failed to get ip", "error", err)
		return
	}
	sdk := k8s.NewK8sClient()
	_, err = sdk.GetConfigCRD(context.Background(), k8s.DomainParseConfigGVR, k8s.DomainParseConfigName)
	if err != nil {
		if errors.IsNotFound(err) {
			config := k8s.NewDomainParseConfigCRD(k8s.DomainParseConfigName, k8s.DomainParseConfigCRDSpec{
				Type: "A",
				IPs:  []string{ip},
			})
			_, err = sdk.DynamicClient().Resource(k8s.DomainParseConfigGVR).Create(context.Background(), config, metav1.CreateOptions{})
			if err != nil {
				slog.Error("Failed to create domain parse config", "error", err)
			}
		}
	}
}
