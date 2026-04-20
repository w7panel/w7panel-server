package console

import (
	"context"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

/*
*
k3k 集群维护模式
*/
func (c Weihu) HandleK3k(clusterName, namespace string) {
	sdk := k8s.NewK8sClient()
	pods, err := sdk.ClientSet.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "cluster=" + clusterName,
	})
	if err != nil {
		slog.Error("获取k3k pod 失败 重试中", "err", err)
		return
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			slog.Info("开始维护模式", "pod", pod.Name)
			err := sdk.ClientSet.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				slog.Error("删除pod失败", "pod", pod.Name, "err", err)
			}
		}
	}
}
func (c Weihu) clearHandleK3k(clusterName, namespace string) {
	sdk := k8s.NewK8sClient()
	pods, err := sdk.ClientSet.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "cluster=" + clusterName,
	})
	if err != nil {
		slog.Error("获取k3k pod 失败 重试中", "err", err)
		return
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			slog.Info("开始维护模式", "pod", pod.Name)
			err := sdk.ClientSet.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				slog.Error("删除pod失败", "pod", pod.Name, "err", err)
			}
		}
	}
}
