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

func (c *Weihu) GetPvcName() string {
	return "varlibrancherk3s-" + c.namespace + "-server-0"
}

// 删除非维护模式pod
func (c *Weihu) ClearNoWeihuPod() error {
	pods, err := c.sdk.ClientSet.CoreV1().Pods(c.namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "cluster=" + c.namespace,
	})
	if err != nil {
		slog.Error("获取k3k pod 失败 重试中", "err", err)
		return err
	}
	for _, pod := range pods.Items {
		if pod.Labels == nil {
			continue
		}
		val, ok := pod.Labels["w7.cc/weihu"]
		if !ok || val != "true" { //删除非维护模式的pod
			err := c.sdk.ClientSet.CoreV1().Pods(c.namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
			if err != nil {
				slog.Error("删除pod失败", "pod", pod.Name, "err", err)
				return err
			}
		}
	}
	return nil
}

func (c *Weihu) ClearTicket() error {
	pvcName, err := c.sdk.ClientSet.CoreV1().PersistentVolumeClaims(c.namespace).Get(context.TODO(), c.GetPvcName(), metav1.GetOptions{})
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
	_, err = longhornClient.ClearVolumeAttachmentTicket(volumeAttachment)
	if err != nil {
		slog.Error("清除volumeAttachmentTicket失败", "volumeName", volumeName, "err", err)
		return err
	}
	return nil
}
