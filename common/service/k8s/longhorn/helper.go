package longhorn

import (
	"fmt"
	"log/slog"
	"sort"

	longhornV1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/samber/lo"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func updateAllReplicaCount() error {
	nodes, err := lclient.GetNodeList()
	if err != nil {
		return err
	}
	return updateAllReplicaCountByNodesList(nodes)
}

func getDiskSelector(node *longhornV1beta2.Node) []string {
	diskSelector := []string{}
	for _, disk := range node.Spec.Disks {
		diskSelector = append(diskSelector, disk.Tags...)
	}
	return diskSelector
}
func getAllDiskSelector(nodes *longhornV1beta2.NodeList) []string {
	diskSelectors := []string{}
	for _, node := range nodes.Items {
		diskSelector := getDiskSelector(&node)
		diskSelectors = append(diskSelectors, diskSelector...)
	}
	return diskSelectors
}
func updateAllReplicaCountByNodesList(nodes *longhornV1beta2.NodeList) error {
	// nodes, err := api.GetNodeList()
	// if err != nil {
	// 	return err
	// }
	volumes, err := lclient.GetVolumeList()
	if err != nil {
		return err
	}
	if len(volumes.Items) == 0 {
		return fmt.Errorf("no nodes found")
	}
	for _, volume := range volumes.Items {
		l := NewlongNodeVolumes(nodes, &volume)
		count, err := l.NeedReplicaCount()
		slog.Info("volume %s will have replicas count: %d=%d", volume.Name, count, "scount", volume.Spec.NumberOfReplicas)
		if err != nil {
			slog.Error("error get replica count for volume %s: %v", volume.Name, err)
			continue
		}
		if count == 0 {
			slog.Info("count = 0 ", "volume", volume.Name)
			continue
		}
		if volume.Spec.NumberOfReplicas != count {
			updateVolumeReplicaCountApi(volume.Name, count)
		}
	}
	return nil
}

func updateNodeLabel() error {
	replicaList, err := lclient.GetLonghornReplicaList()
	if err != nil {
		slog.Error("replicaList error", "err", err)
		return err
	}
	k8sNodes, err := lclient.GetK8sNodeList()
	if err != nil {
		slog.Error("k8sNodes error", "err", err)
		return err
	}
	for _, k8snode := range k8sNodes.Items {
		labels := k8snode.Labels
		longhornNode := GetLonghornReplicaByNodeName(replicaList, k8snode.Name)
		if longhornNode != nil {
			//允许调度 并且没驱除
			if labels == nil {
				labels = map[string]string{}
			}
			_, ok := labels["node-role.kubernetes.io/storage"]
			if !ok {
				labels["node-role.kubernetes.io/storage"] = "true"
				_, err2 := lclient.UpdateK8sNodeLabel(&k8snode, labels)
				if err2 != nil {
					slog.Error("UpdateK8sNodeLabel error", "err2", err2)
					return err2
				}
			}
		} else {
			_, ok := labels["node-role.kubernetes.io/storage"]
			if ok {
				delete(labels, "node-role.kubernetes.io/storage")
				_, err3 := lclient.UpdateK8sNodeLabel(&k8snode, labels)
				if err3 != nil {
					slog.Error("UpdateK8sNodeLabel error", "err3", err3)
					return err3
				}
			}
		}
	}
	return nil
}

func GetLonghornNodeByNodeName(list *longhornV1beta2.NodeList, nodeName string) *longhornV1beta2.Node {
	for _, node := range list.Items {
		if node.Name == nodeName {
			return &node
		}
	}
	return nil
}

func GetLonghornReplicaByNodeName(list *longhornV1beta2.ReplicaList, nodeName string) *longhornV1beta2.Replica {
	for _, node := range list.Items {
		if node.Spec.NodeID == nodeName {
			return &node
		}
	}
	return nil
}

/**
* Description: 更新副本数
* @param name
 */
func VolumeUpdateReplicaCount(sdk *k8s.Sdk, name string) error {

	api, err := NewLonghornClient(sdk)
	if err != nil {
		return err
	}
	volume, err := api.GetVolume(name)
	if err != nil {
		return err
	}
	selector := volume.Spec.DiskSelector
	if selector == nil {
		selector = []string{}
	}
	willCount, err := api.GetDisksCount(selector)
	if err != nil {
		return err
	}
	if willCount == 0 {
		return fmt.Errorf("no disk found for volume %s", name)
	}
	// if (willCount) != volume.Spec.NumberOfReplicas {
	// 	volume.Spec.NumberOfReplicas = willCount
	// }
	slog.Info("volume %s will have replicas count: %d", name, willCount)
	updateVolumeReplicaCountApi(name, willCount)
	return err
}

func containsAll(a, b []string) bool {
	for _, item := range b {
		found := false
		for _, elem := range a {
			if elem == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// 快照大小
func GetSnapshopSize(volumeName string, snapList *longhornV1beta2.SnapshotList) int64 {
	var size int64
	for _, snap := range snapList.Items {
		if snap.Spec.Volume == volumeName {
			size += snap.Status.Size
		}
	}
	return size
}

// 是否扩容中 longhorn ui的逻辑
func IsVolumeExpanding(volume *longhornV1beta2.Volume, eList *longhornV1beta2.EngineList) (bool, string) {
	filterList := lo.Filter(eList.Items, func(eg longhornV1beta2.Engine, index int) bool {
		return eg.Labels["longhornvolume"] == volume.Name
	})
	egSort := lo.KeyBy(filterList, func(eg2 longhornV1beta2.Engine) string {
		return eg2.Name
	})
	keys := lo.Keys(egSort)
	sort.Strings(keys)
	for _, v := range keys {
		eg, ok := egSort[v]
		if !ok {
			slog.Error("volume %s is not expanding", "name", volume.Name)
			continue
		}
		if ok && volume.Spec.Size != eg.Status.CurrentSize && volume.Status.State == "attached" {
			slog.Error("volume %s is expanding", "name", volume.Name)
			return true, eg.Status.LastExpansionError
		}
	}

	return false, ""
}

func IsVolumeLock(volume *longhornV1beta2.Volume, eList *longhornV1beta2.VolumeAttachmentList) (bool, string) {
	vtList := lo.Filter(eList.Items, func(eg longhornV1beta2.VolumeAttachment, index int) bool {
		return eg.Spec.Volume == volume.Name
	})
	if len(vtList) > 0 {
		first := vtList[0]
		if len(first.Spec.AttachmentTickets) > 1 {
			for _, ticket := range first.Spec.AttachmentTickets {
				if ticket.ID == "longhorn-ui" {
					continue
				}
				return true, ticket.NodeID
			}
		}
	}
	return false, ""
}
func VolumeAttachNodeId(volume *longhornV1beta2.Volume, eList *longhornV1beta2.VolumeAttachmentList) string {
	vtList := lo.Filter(eList.Items, func(eg longhornV1beta2.VolumeAttachment, index int) bool {
		return eg.Spec.Volume == volume.Name
	})
	if len(vtList) > 0 {
		first := vtList[0]
		for _, ticket := range first.Spec.AttachmentTickets {
			return ticket.NodeID
		}
	}
	return ""
}
