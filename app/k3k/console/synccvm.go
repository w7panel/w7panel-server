package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	corev1 "k8s.io/api/core/v1"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type SyncCvm struct {
	console2.Abstract
}

func (c SyncCvm) GetName() string {
	return "sync-cvm"
}

func (c SyncCvm) Configure(cmd *cobra.Command) {

}

func (c SyncCvm) GetDescription() string {
	return "用户集群转cvm"
}

func (c SyncCvm) Handle(cmd *cobra.Command, args []string) {

	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("Failed to create sigclient", "error", err)
	}
	list := &corev1.ServiceAccountList{}
	err = sigClient.List(sdk.Ctx, list, &sigclient.ListOptions{Namespace: "default"})
	if err != nil {
		slog.Error("Failed to list cluster list", "error", err)
	}
	for _, sa := range list.Items {
		user := k3ktypes.NewK3kUser(&sa)
		if !user.IsOldClusterUser() {
			continue
		}
		slog.Info("start Sync user to cvm", "username", user.Name)
		err := k3k.SyncUserToCvm(context.Background(), user, sdk.Sdk)
		if err != nil {
			slog.Error("Failed to sync user to cvm", "error", err, "username", user.Name)
		}

	}
}
