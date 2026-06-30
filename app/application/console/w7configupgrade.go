package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	configservice "github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type W7ConfigUpgrade struct {
	console2.Abstract
}

func (c W7ConfigUpgrade) GetName() string {
	return "w7config-upgrade"
}

func (c W7ConfigUpgrade) Configure(cmd *cobra.Command) {}

func (c W7ConfigUpgrade) GetDescription() string {
	return "升级 w7-config Secret 到 User CRD"
}

func (c W7ConfigUpgrade) Handle(cmd *cobra.Command, args []string) {
	sdk := k8s.NewK8sClient().Sdk
	repo := configservice.NewW7ConfigRepository(sdk)
	if err := repo.MigrateSecretsToUsers(context.Background()); err != nil {
		slog.Error("升级 w7-config 失败", "error", err)
		return
	}
	slog.Info("升级 w7-config 完成")
}
