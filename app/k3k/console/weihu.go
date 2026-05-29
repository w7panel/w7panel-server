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
	corev1 "k8s.io/api/core/v1"
)

type Weihu struct {
	console2.Abstract
}
type weihuOp struct {
	clusterName  string
	k3kNamespace string
<<<<<<< HEAD
	cvmName      string
=======
>>>>>>> dev-v1
}

var whOp = weihuOp{}

func (c Weihu) GetName() string {
	return "weihu"
}

func (c Weihu) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&whOp.clusterName, "clusterName", "", "clusterName")
	cmd.Flags().StringVar(&whOp.k3kNamespace, "k3knamespace", "", "namespace")
<<<<<<< HEAD
	cmd.Flags().StringVar(&whOp.k3kNamespace, "cvmName", "", "cvmName")
=======
>>>>>>> dev-v1
}

func (c Weihu) GetDescription() string {
	return "救援模式job"
}

func (c Weihu) Handle(cmd *cobra.Command, args []string) {
<<<<<<< HEAD
	c.HandleK3k(whOp.clusterName, whOp.k3kNamespace, whOp.cvmName)
=======
	c.HandleK3k(whOp.clusterName, whOp.k3kNamespace)
>>>>>>> dev-v1
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
<<<<<<< HEAD
func (c Weihu) HandleK3k(clusterName, namespace, cvmName string) {
	sdk := k8s.NewK8sClient().Sdk
	wh := k3k.NewWeihu(sdk, clusterName, namespace, cvmName)
=======
func (c Weihu) HandleK3k(clusterName, namespace string) {
	sdk := k8s.NewK8sClient().Sdk
	wh := k3k.NewWeihu(sdk, clusterName, namespace)
>>>>>>> dev-v1
	//controll runtime 重试3次
	ctx := context.Background()
	retry := 3
	//设置超时时间5秒
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)
	defer cancel()
	err := helper.Retry(func() error {
		return wh.ClearPod(ctx)
	}, retry, time.Second*10)
	if err != nil {
		slog.Error("清理非维护模式pod err, 请重试", "error", err)
		os.Exit(1)
		return
	}

	err = helper.Retry(func() error {
		return wh.TrimFilesystem(ctx)
	}, 3, time.Second*5)
	if err != nil {
		slog.Error("清理longhorn trimSystem err", "error", err)
		// 不退出
	}
	slog.Info("清理longhorn volume ticket")
	err = wh.ClearTicket(ctx)
	if err != nil {
		slog.Error("清理longhorn volume ticket err", "error", err)
		// 不退出
	}

	var whPod *corev1.Pod
	for i := 0; i < retry; i++ {
		whPod, err = wh.GetWeihuingPod(ctx)
		if err != nil {
			slog.Error("获取维护模式pod err", "error", err, "retry", retry)
			time.Sleep(time.Second * 10)
			continue
		}
	}
	if whPod == nil {
		slog.Error("获取维护pod err", "error", err)
		os.Exit(1)
		return
	}
	if whPod.Status.Phase != corev1.PodRunning {
		//尝试修复 not running pod
		slog.Info("维护pod 非running, 尝试修复")
		err = wh.TryFixNotRunningPod(ctx, whPod)
		if err != nil {
			slog.Error("尝试修复 pod err", "error", err)
			os.Exit(1)
			return
		}
	}
	time.Sleep(time.Second * 5)
	//刷新pod 状态 检查是否启动
	for i := 0; i < retry; i++ {
		whPod, err = wh.RefreshPod(ctx, whPod)
		if err != nil {
			slog.Error("刷新pod err", "error", err)
			time.Sleep(time.Second * 15)
			continue
		}
		if whPod.Status.Phase == corev1.PodRunning {
			slog.Info("救援pod 启动成功")
			break
		}
		time.Sleep(time.Second * 15)
	}
	if whPod.Status.Phase != corev1.PodRunning {
		slog.Error("维护模式pod 启动失败, 请重试")
		os.Exit(1)
		return
	}

	// slog.Info("等待30秒,检查集群是否正常")
	// time.Sleep(time.Second * 30)
	// err = helper.RetryFullSuccess(func() error {
	// 	return wh.CheckOk(ctx)
	// }, 3, time.Second*5)
	// if err != nil {
	// 	slog.Error("集群访问异常，请重试", "error", err)
	// 	os.Exit(1)
	// 	return
	// }
	slog.Info("集群救援结束, 等待集群启动中...")
}
