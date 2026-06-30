package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	userservice "github.com/w7panel/w7panel/common/service/user"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type UserUpgrade struct {
	console2.Abstract
}

func (c UserUpgrade) GetName() string {
	return "user-upgrade"
}

func (c UserUpgrade) Configure(cmd *cobra.Command) {}

func (c UserUpgrade) GetDescription() string {
	return "升级 ServiceAccount 用户到 User CRD"
}

func (c UserUpgrade) Handle(cmd *cobra.Command, args []string) {
	sdk := k8s.NewK8sClient().Sdk
	if err := userservice.MigrateServiceAccounts(context.Background(), sdk); err != nil {
		slog.Error("升级用户失败", "error", err)
		return
	}
	slog.Info("升级用户完成")
}
