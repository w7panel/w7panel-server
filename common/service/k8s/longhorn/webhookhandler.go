package longhorn

import (
	"context"
	"log/slog"
	"time"

	"strings"

	longhornV1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
)

var lclient *longhornclient

func init() {
	lclient, _ = NewLonghornClient(k8s.NewK8sClientInner())
}

func OnStart() error {
	k8sNodes, err := lclient.GetK8sNodeList()
	if err != nil {
		slog.Error("longhorn get k8s node list error", "error", err)
		return err
	}
	nodes, nodeErr := lclient.GetNodeList()
	diskCreated := make(map[string]bool)
	if nodeErr == nil {
		for _, node := range nodes.Items {
			if _, ok := node.Spec.Disks["disk-default"]; ok {
				diskCreated[node.Name] = true
			}
		}
	}
	for _, node := range k8sNodes.Items {
		if node.Labels[defaultDiskLabel] != "config" || !isDefaultDiskConfig(node.Annotations[defaultDiskAnnotation]) {
			continue
		}
		if err := clearDefaultDiskNode(lclient.sdk, node.Name, diskCreated[node.Name], false); err != nil {
			slog.Error("cleanup Longhorn default disk node config error", "node", node.Name, "error", err)
		}
	}
	if nodeErr != nil {
		slog.Error("longhorn get node list error", "error", nodeErr)
		return nodeErr
	}

	// createDefaultVolumeByNodeList(nodes)
	updateAllReplicaCountByNodesList(nodes)
	createDefaultStorageClassByPvc()
	deleteNotSelectorStorageClass(nodes)
	return nil
}

func deleteNotSelectorStorageClass(nodes *longhornV1beta2.NodeList) {
	scs, err := lclient.GetK8sStorageClassList()
	if err != nil {
		slog.Error("longhorn get pvc list error", "error", err)
		return
	}
	scNames := []string{}
	for _, sc := range scs.Items {
		scNames = append(scNames, sc.Name)
	}
	diskSelector := getAllDiskSelector(nodes)
	for _, scName := range scNames {
		if !containsArr(diskSelector, scName) && (strings.HasPrefix(scName, "union") || strings.HasPrefix(scName, "disk")) {
			err := lclient.DeleteK8sStorageClass(scName)
			if err != nil {
				slog.Error("longhorn delete storage class error", "error", err)
			}
		}
	}
}

func WebhookLonghornNode(node *longhornV1beta2.Node) error {
	time.AfterFunc(time.Second*5, func() {
		if _, ok := node.Spec.Disks["disk-default"]; ok {
			if err := clearDefaultDiskNode(lclient.sdk, node.Name, true, false); err != nil {
				slog.Error("cleanup Longhorn default disk node config error", "node", node.Name, "error", err)
			}
		}
		createStorageClass(node)
		nodes, err := lclient.GetNodeList()
		if err != nil {
			slog.Error("longhorn get node list error", "error", err)
			return
		}
		// createDefaultVolumeByNodeList(nodes)
		updateAllReplicaCountByNodesList(nodes)
		createDefaultStorageClassByPvc()
		deleteNotSelectorStorageClass(nodes)
	})
	return nil
}

func WebHookStorageClass(storageClass *storagev1.StorageClass) bool {
	if storageClass.Name == "longhorn" {
		val, ok := storageClass.Annotations["storageclass.kubernetes.io/is-default-class"]
		if ok && val == "true" {
			storageClass.Annotations["storageclass.kubernetes.io/is-default-class"] = "false"
			return true
		}
	}
	return false
}

func WebHookLonghornReplica() {
	time.AfterFunc(time.Second*5, func() {
		updateNodeLabel()
		updateAllReplicaCount() //更新副本数
	})

}

func WebHookDeleteNode(node *v1.Node) {
	time.AfterFunc(time.Second*1, func() {
		if lclient == nil {
			slog.Error("longhorn delete node skipped: client is not initialized", "node", node.Name)
			return
		}
		if err := lclient.DeleteNodeWhenReady(context.Background(), node.Name, 30*time.Second); err != nil {
			slog.Error("longhorn delete node error", "node", node.Name, "error", err)
		}
	})
}

func GetLonghornNodeList() (*longhornV1beta2.NodeList, error) {
	return lclient.GetNodeList()
}
