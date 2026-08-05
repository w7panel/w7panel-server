package longhorn

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"

	longhornV1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/samber/lo"
	"github.com/w7panel/w7panel/common/service/k8s"
)

type VolumeSnapshotInfo struct {
	Name        string
	Size        int64
	Created     string
	Removed     bool
	UserCreated bool
}

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

// GetVolumeSnapshots returns the snapshot layers that currently occupy the
// volume's engine chain. volume-head is writable data rather than a snapshot
// and is deliberately excluded. A removed layer is kept until Longhorn purges
// it because it can still consume physical storage.
func GetVolumeSnapshots(volumeName string, snapList *longhornV1beta2.SnapshotList, engineList *longhornV1beta2.EngineList) []VolumeSnapshotInfo {
	if engine := selectVolumeEngine(volumeName, engineList); engine != nil && engine.Status.Snapshots != nil {
		result := make([]VolumeSnapshotInfo, 0, len(engine.Status.Snapshots))
		for key, snapshot := range engine.Status.Snapshots {
			if snapshot == nil {
				continue
			}
			name := snapshot.Name
			if name == "" {
				name = key
			}
			if name == "volume-head" {
				continue
			}
			size, err := strconv.ParseInt(snapshot.Size, 10, 64)
			if err != nil {
				slog.Warn("invalid longhorn engine snapshot size", "volumeName", volumeName, "snapshotName", name, "size", snapshot.Size, "err", err)
			}
			result = append(result, VolumeSnapshotInfo{
				Name:        name,
				Size:        size,
				Created:     snapshot.Created,
				Removed:     snapshot.Removed,
				UserCreated: snapshot.UserCreated,
			})
		}
		sortVolumeSnapshots(result)
		return result
	}

	result := make([]VolumeSnapshotInfo, 0)
	if snapList == nil {
		return result
	}
	for _, snapshot := range snapList.Items {
		if snapshot.Spec.Volume != volumeName {
			continue
		}
		result = append(result, VolumeSnapshotInfo{
			Name:        snapshot.Name,
			Size:        snapshot.Status.Size,
			Created:     snapshot.Status.CreationTime,
			Removed:     snapshot.Status.MarkRemoved,
			UserCreated: snapshot.Status.UserCreated,
		})
	}
	sortVolumeSnapshots(result)
	return result
}

func GetSnapshotSize(volumeName string, snapList *longhornV1beta2.SnapshotList, engineList *longhornV1beta2.EngineList) int64 {
	var size int64
	for _, snapshot := range GetVolumeSnapshots(volumeName, snapList, engineList) {
		size += snapshot.Size
	}
	return size
}

func selectVolumeEngine(volumeName string, engineList *longhornV1beta2.EngineList) *longhornV1beta2.Engine {
	if engineList == nil {
		return nil
	}
	candidates := make([]*longhornV1beta2.Engine, 0)
	for i := range engineList.Items {
		engine := &engineList.Items[i]
		if engine.Spec.VolumeName == volumeName || engine.Labels["longhornvolume"] == volumeName {
			candidates = append(candidates, engine)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank := engineSelectionRank(candidates[i])
		rightRank := engineSelectionRank(candidates[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return candidates[i].Name < candidates[j].Name
	})
	if len(candidates) == 0 {
		return nil
	}
	return candidates[0]
}

func engineSelectionRank(engine *longhornV1beta2.Engine) int {
	running := string(engine.Status.CurrentState) == "running"
	if engine.Spec.Active && running {
		return 0
	}
	if engine.Spec.Active {
		return 1
	}
	if running {
		return 2
	}
	return 3
}

func sortVolumeSnapshots(snapshots []VolumeSnapshotInfo) {
	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].Created != snapshots[j].Created {
			return snapshots[i].Created > snapshots[j].Created
		}
		return snapshots[i].Name < snapshots[j].Name
	})
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

// VolumeAttachmentState returns the requested attachment direction instead of
// relying only on Volume.Status.State, which can lag behind ticket changes.
func VolumeAttachmentState(volume *longhornV1beta2.Volume, eList *longhornV1beta2.VolumeAttachmentList) string {
	hasAttachmentTicket := false
	for _, attachment := range eList.Items {
		if attachment.Spec.Volume == volume.Name && len(attachment.Spec.AttachmentTickets) > 0 {
			hasAttachmentTicket = true
			break
		}
	}

	state := string(volume.Status.State)
	if hasAttachmentTicket {
		if state == "attached" {
			return "attached"
		}
		return "attaching"
	}
	if state == "detached" {
		return "detached"
	}
	if state == "attached" || state == "attaching" || state == "detaching" {
		return "detaching"
	}
	return state
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
