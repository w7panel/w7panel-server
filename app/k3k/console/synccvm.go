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
	"k8s.io/apimachinery/pkg/types"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type SyncCvm struct {
	console2.Abstract
}

type cvmOp struct {
	username string
}

var cOp = cvmOp{}

func (c SyncCvm) GetName() string {
	return "sync-cvm"
}

func (c SyncCvm) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&cOp.username, "username", "", "用户名")
}

func (c SyncCvm) GetDescription() string {
	return "用户集群转cvm"
}
func (c SyncCvm) Handle(cmd *cobra.Command, args []string) {

	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("Failed to create sigclient", "error", err)
		return
	}
	if cOp.username != "" {
		c.HandleOne(sigClient, sdk.Sdk, cOp.username)
	} else {
		c.HandleList(sigClient, sdk.Sdk)
	}
}
func (c SyncCvm) HandleList(sigClient sigclient.Client, sdk *k8s.Sdk) {

	list := &corev1.ServiceAccountList{}
	err := sigClient.List(sdk.Ctx, list, &sigclient.ListOptions{Namespace: "default"})
	if err != nil {
		slog.Error("Failed to list cluster list", "error", err)
	}
	for _, sa := range list.Items {
		err = syncCvm(sigClient, &sa, sdk)
		if err != nil {
			slog.Error("Failed to sync cvm", "error", err)
		}

	}
}

func (c SyncCvm) HandleOne(sigClient sigclient.Client, sdk *k8s.Sdk, username string) {

	sa := &corev1.ServiceAccount{}
	err := sigClient.Get(sdk.Ctx, types.NamespacedName{Namespace: "default", Name: username}, sa)
	if err != nil {
		slog.Error("Failed to get sa", "error", err)
		return
	}
	err = syncCvm(sigClient, sa, sdk)
	if err != nil {
		slog.Error("Failed to sync cvm", "error", err)
	}
}

func syncCvm(client sigclient.Client, sa *corev1.ServiceAccount, sdk *k8s.Sdk) error {
	user := k3ktypes.NewK3kUser(sa)
	if !user.IsOldClusterUser() {
		slog.Info("user is not old cluster user", "username", user.Name)
		return nil
	}
	slog.Info("start Sync user to cvm", "username", user.Name)
	err := k3k.SyncUserToCvm(context.Background(), user, sdk)
	if err != nil {
		slog.Error("Failed to sync user to cvm", "error", err, "username", user.Name)
	}
	return err
}
