package console

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type Weihu struct {
	console2.Abstract
}
type weihuOp struct {
	clusterName  string
	k3kNamespace string
}

var whOp = weihuOp{}

func (c Weihu) GetName() string {
	return "weihu"
}

func (c Weihu) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&whOp.clusterName, "clusterName", "", "clusterName")
	cmd.Flags().StringVar(&whOp.k3kNamespace, "k3knamespace", "", "namespace")
}

func (c Weihu) GetDescription() string {
	return "维护模式job"
}

func (c Weihu) Handle(cmd *cobra.Command, args []string) {
	c.HandleK3k(whOp.clusterName, whOp.k3kNamespace)
}

/**
Events:
  Type     Reason       Age                  From               Message
  ----     ------       ----                 ----               -------
  Normal   Scheduled    9m8s                 default-scheduler  Successfully assigned k3k-console-164315/k3k-console-164315-server-0 to server1
  Warning  FailedMount  49s (x12 over 9m3s)  kubelet            MountVolume.MountDevice failed for volume "pvc-ce0e697a-d6a4-4136-a366-a638618fd9e8" : rpc error: code = InvalidArgument desc = volume pvc-ce0e697a-d6a4-4136-a366-a638618fd9e8 hasn't been attached yet


*/
/*
*
k3k 集群维护模式
*/
func (c Weihu) HandleK3k(clusterName, namespace string) {
	sdk := k8s.NewK8sClient().Sdk
	wh := k3k.NewWeihu(sdk, clusterName, namespace)
	//controll runtime 重试3次
	ctx := context.Background()
	//设置超时时间5秒
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()
	err := helper.Retry(func() error {
		return wh.ClearNoWeihuPod(ctx)
	}, 3, time.Second*5)
	if err != nil {
		slog.Error("清理非维护模式pod err", "error", err)
		os.Exit(1)
		return
	}

	err = helper.Retry(func() error {
		return wh.ClearTicket(ctx)
	}, 3, time.Second*5)
	if err != nil {
		slog.Error("清理longhorn ticket err", "error", err)
		os.Exit(1)
		return
	}
	slog.Info("等待30秒,检查集群是否正常")
	time.Sleep(time.Second * 30)
	err = helper.RetryFullSuccess(func() error {
		return wh.CheckOk(ctx)
	}, 3, time.Second*5)
	if err != nil {
		slog.Error("检查集群异常", "error", err)
		os.Exit(1)
		return
	}
}
