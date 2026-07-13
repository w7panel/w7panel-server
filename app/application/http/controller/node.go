package controller

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Nodes struct {
	controller.Abstract
}

type longhornReplicaCleanupItem struct {
	VolumeName          string `json:"volumeName"`
	ReplicaName         string `json:"replicaName"`
	NodeName            string `json:"nodeName"`
	VolumeState         string `json:"volumeState"`
	VolumeRobustness    string `json:"volumeRobustness"`
	AttachedNode        string `json:"attachedNode"`
	HealthyReplicaCount int    `json:"healthyReplicaCount"`
	ForceRequired       bool   `json:"forceRequired"`
}

// longhornReplicaItems returns only replicas whose current node is the node
// being removed. A healthy replica has completed a rebuild and is not failed.
func (self Nodes) longhornReplicaItems(http *gin.Context) ([]longhornReplicaCleanupItem, error) {
	sdk, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		return nil, err
	}
	client, err := longhorn.NewLonghornClient(sdk)
	if err != nil {
		return nil, err
	}
	volumes, err := client.GetVolumeList()
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []longhornReplicaCleanupItem{}, nil
		}
		return nil, err
	}
	replicas, err := client.GetReplicaList()
	if err != nil {
		if apierrors.IsNotFound(err) {
			return []longhornReplicaCleanupItem{}, nil
		}
		return nil, err
	}

	type volumeInfo struct{ state, robustness, attached string }
	volumeByName := make(map[string]volumeInfo, len(volumes.Items))
	for _, volume := range volumes.Items {
		volumeByName[volume.Name] = volumeInfo{
			state: string(volume.Status.State), robustness: string(volume.Status.Robustness), attached: volume.Status.CurrentNodeID,
		}
	}
	healthyCount := make(map[string]int)
	for _, replica := range replicas.Items {
		if replica.Spec.HealthyAt != "" && replica.Spec.FailedAt == "" {
			healthyCount[replica.Labels["longhornvolume"]]++
		}
	}

	items := []longhornReplicaCleanupItem{}
	for _, replica := range replicas.Items {
		if replica.Spec.NodeID != http.Param("name") {
			continue
		}
		volumeName := replica.Labels["longhornvolume"]
		volume, ok := volumeByName[volumeName]
		if !ok || volumeName == "" {
			continue
		}
		isHealthy := replica.Spec.HealthyAt != "" && replica.Spec.FailedAt == ""
		items = append(items, longhornReplicaCleanupItem{
			VolumeName: volumeName, ReplicaName: replica.Name, NodeName: replica.Spec.NodeID,
			VolumeState: volume.state, VolumeRobustness: volume.robustness, AttachedNode: volume.attached,
			HealthyReplicaCount: healthyCount[volumeName], ForceRequired: isHealthy && healthyCount[volumeName] <= 1,
		})
	}
	return items, nil
}

func (self Nodes) GetLonghornReplicas(http *gin.Context) {
	items, err := self.longhornReplicaItems(http)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, items)
}

func (self Nodes) DeleteLonghornReplicas(http *gin.Context) {
	params := struct{ Force bool `json:"force"` }{}
	if err := http.ShouldBindJSON(&params); err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	items, err := self.longhornReplicaItems(http)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	for _, item := range items {
		if item.ForceRequired && !params.Force {
			self.JsonResponseWithServerError(http, errors.New("存在唯一健康副本，确认风险后才能删除"))
			return
		}
	}

	sdk, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	client, err := longhorn.NewLonghornClient(sdk)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	node, err := client.GetNode(http.Param("name"))
	if err == nil {
		node.Spec.AllowScheduling = false
		if _, err = client.UpdateNode(node); err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
	} else if !apierrors.IsNotFound(err) {
		self.JsonResponseWithServerError(http, err)
		return
	}
	for _, item := range items {
		if err := longhorn.RemoveReplica(item.VolumeName, item.ReplicaName); err != nil {
			self.JsonResponseWithServerError(http, err)
			return
		}
	}
	self.JsonResponseWithoutError(http, gin.H{"items": items})
}

func (self Nodes) Delete(http *gin.Context) {
	items, err := self.longhornReplicaItems(http)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if len(items) > 0 {
		self.JsonResponseWithServerError(http, errors.New("Longhorn 副本尚未清理完成"))
		return
	}

	name := http.Param("name")
	sdk, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	client, err := longhorn.NewLonghornClient(sdk)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if err := client.DeleteNode(name); err != nil && !apierrors.IsNotFound(err) {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if err := sdk.ClientSet.CoreV1().Nodes().Delete(sdk.Ctx, name, metav1.DeleteOptions{}); err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, gin.H{"name": name})
}
