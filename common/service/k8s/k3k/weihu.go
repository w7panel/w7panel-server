package k3k

import (
	"context"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Weihu struct {
	sdk         *k8s.Sdk
	clusterName string
	namespace   string
}

func NewWeihu(sdk *k8s.Sdk, clusterName, namespace string) *Weihu {
	return &Weihu{sdk: sdk, clusterName: clusterName, namespace: namespace}
}
func (c *Weihu) GetPvcName() string {
	return "varlibrancherk3s-" + c.namespace + "-server-0"
}

// 删除非维护模式pod
func (c *Weihu) ClearNoWeihuPod(ctx context.Context) error {
	pods, err := c.sdk.ClientSet.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "cluster=" + c.namespace,
	})
	if err != nil {
		if k8errors.IsNotFound(err) {
			slog.Info("非维护模式 pod 不存在, 无需清理")
			return nil
		}
		slog.Error("获取k3k pod 失败 重试中", "err", err)
		return err
	}
	for _, pod := range pods.Items {
		if pod.Labels == nil {
			continue
		}
		val, ok := pod.Labels["w7.cc/weihu"]
		if !ok || val != "true" { //删除非维护模式的pod //k3k pod 会有finalizers
			if pod.DeletionTimestamp != nil { //正在删除的pod
				pod.Finalizers = []string{}
				_, err := c.sdk.ClientSet.CoreV1().Pods(c.namespace).Update(context.TODO(), &pod, metav1.UpdateOptions{})
				if err != nil {
					slog.Error("更新pod失败", "pod", pod.Name, "err", err)
					return err
				}
			} else {
				err := c.sdk.ClientSet.CoreV1().Pods(c.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
				if err != nil {
					slog.Error("删除pod失败", "pod", pod.Name, "err", err)
					return err
				}
			}
		}
	}
	return nil
}

func (c *Weihu) ClearTicket(ctx context.Context) error {
	pvcName, err := c.sdk.ClientSet.CoreV1().PersistentVolumeClaims(c.namespace).Get(ctx, c.GetPvcName(), metav1.GetOptions{})
	if err != nil {
		slog.Error("获取pvc失败", "pvc", c.GetPvcName(), "err", err)
		return err
	}
	if pvcName.Spec.VolumeName == "" {
		return nil
	}
	volumeName := pvcName.Spec.VolumeName
	longhornClient, err := longhorn.NewLonghornClient(c.sdk)
	if err != nil {
		slog.Error("获取longhornClient失败", "err", err)
		return err
	}
	volumeAttachment, err := longhornClient.GetVolumeAttachment(volumeName)
	if err != nil {
		if k8errors.IsNotFound(err) { //可能没使用longhorn 存储
			slog.Info("volumeAttachment不存在", "volumeName", volumeName)
			return nil
		}
		slog.Error("获取volumeAttachment失败", "volumeName", volumeName, "err", err)
		return err
	}
	if len(volumeAttachment.Spec.AttachmentTickets) == 0 {
		slog.Info("volumeAttachment不存在ticket, 无需清理")
		return nil
	}
	if len(volumeAttachment.Spec.AttachmentTickets) > 0 {
		slog.Info("volumeAttachment存在ticket, 清除...", "volumeName", volumeName)
		_, err = longhornClient.ClearVolumeAttachmentTicket(volumeAttachment)
		if err != nil {
			slog.Error("清除volumeAttachmentTicket失败", "volumeName", volumeName, "err", err)
			return err
		}
	}

	slog.Info("优化存储Longhorn TrimFilesystem...")
	err = longhorn.LonghornVolumeTrimFilesystem(volumeName)
	if err != nil {
		slog.Error("trim volume失败", "volumeName", volumeName, "err", err)
		return err
	}
	return nil
}

func (c *Weihu) CheckOk(ctx context.Context) error {
	k3kconfig := k8s.NewK3kConfig(c.clusterName, c.namespace, "")
	sdk := k8s.NewK8sClient()
	sdk.Clear(c.clusterName)
	client, err := sdk.GetK3kClusterSdkByConfig(k3kconfig)
	if err != nil {
		slog.Error("获取k3k集群失败", "err", err)
		return err
	}
	//调用集群version 方法
	_, err = client.ClientSet.CoreV1().Namespaces().Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		slog.Error("集群连接性测试失败, 重试中...", "err", err)
		return err
	}
	return nil

}
