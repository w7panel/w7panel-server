package console

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type SiteZpk struct {
	console2.Abstract
}

type siteZpkOption struct {
	ThirdPartyCDToken string
	Host              string
	ReleaseName       string
	SiteIdentifie     string
	AppName           string
	ContainerName     string
	Namespace         string
}

var siteroZpk = siteZpkOption{}

func (c SiteZpk) GetName() string {
	return "site:register-zpk"
}

func (c SiteZpk) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&siteroZpk.ThirdPartyCDToken, "thirdPartyCDToken", "", "交付系统token")
	cmd.Flags().StringVar(&siteroZpk.Host, "host", "", "域名")
	cmd.Flags().StringVar(&siteroZpk.ReleaseName, "releaseName", "", "安装name")
	cmd.Flags().StringVar(&siteroZpk.SiteIdentifie, "siteIdentifie", "", "站点标识")
	cmd.Flags().StringVar(&siteroZpk.AppName, "appName", "", "deployment名字")
	cmd.Flags().StringVar(&siteroZpk.ContainerName, "containerName", "", "containerName名字")
	cmd.Flags().StringVar(&siteroZpk.Namespace, "namespace", "", "namespace")
}

func (c SiteZpk) GetDescription() string {
	return "站点注册"
}

func (c SiteZpk) Handle(cmd *cobra.Command, args []string) {
	c.registerSite()
}

func (c SiteZpk) registerSite() {

	slog.Info("证书验证成功，开始注册站点...")
	secret, err := console.RegisterSiteZpk(siteroZpk.Host, siteroZpk.SiteIdentifie)
	if err != nil {
		slog.Error("注册站点失败", "err", err)
		os.Exit(1)
	}

	slog.Info("注册站点成功", "secret", secret)
	sdk := k8s.NewK8sClientInner()
	err = console.PatchAppId(sdk, secret, siteroZpk.AppName, siteroZpk.Namespace, siteroZpk.ContainerName)
	if err != nil {
		slog.Error("更新appid失败", "err", err)
		os.Exit(1)
	}

	slog.Info("站点注册完成")
	os.Exit(0)
}
