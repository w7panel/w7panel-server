package controller

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"

	// "github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"

	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Longhorn struct {
	controller.Abstract
}

func (self Longhorn) GetNeedDeleteReplicas(http *gin.Context) {
	type ParamsValidate struct {
		DiskSelector string `form:"diskselector" binding:"required"`
		NodeId       string `form:"nodeid" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	nodeIds := strings.Split(params.NodeId, ",")

	sdk, err := k8s.NewK8sClient().Channel(http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	longhornclient, err := longhorn.NewLonghornClient(sdk)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	compose, err := longhornclient.GetVolumeReplicaCompose()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	replicas := compose.GetVolumeReplicas().GetNeedDeleteReplicas([]string{params.DiskSelector}, nodeIds)
	self.JsonResponseWithoutError(http, replicas)
}

func (self Longhorn) GetVolumesStatus(http *gin.Context) {
	type ParamsValidate struct {
		ConvertPvc string `form:"convertpvc"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	type VolumesStatus struct {
		NumberOfReplicas  int    `json:"numberOfReplicas"`
		Robustness        string `json:"robustness"`
		Size              int64  `json:"size,string"`
		ActualSize        int64  `json:"actualSize"`
		CreationTimestamp string `json:"creationTimestamp"`
		AccessMode        string `json:"accessMode"`
		SnapShotSize      int64  `json:"snapShotSize"`
		IsExpanding       bool   `json:"isExpanding"` //是否在扩容中
		ExpandErr         string `json:"expandErr"`   //扩容失败消息
		State             string `json:"state"`       //volume状态
		VolumeName        string `json:"volumeName"`
	}

	sdk := k8s.NewK8sClient().Sdk
	longhornclient, err := longhorn.NewLonghornClient(sdk)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	volumes, err := longhornclient.GetVolumeList()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	snapList, err := longhornclient.GetSnapshotList()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	engineList, err := longhornclient.GetEngineList()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	// func
	result := map[string]VolumesStatus{}
	for _, volume := range volumes.Items {
		if volume.Status.KubernetesStatus.PVCName == "" {
			continue
		}
		size := volume.Status.ActualSize //已使用空间 /1024/1024/ MB
		isExpanding, expandErrstr := longhorn.IsVolumeExpanding(volume.Name, engineList)
		vs := VolumesStatus{
			NumberOfReplicas:  volume.Spec.NumberOfReplicas,
			Robustness:        string(volume.Status.Robustness),
			Size:              volume.Spec.Size,
			ActualSize:        size,
			AccessMode:        string(volume.Spec.AccessMode),
			CreationTimestamp: volume.CreationTimestamp.Format("2006-01-02 15:04:05"),
			SnapShotSize:      longhorn.GetSnapshopSize(volume.Name, snapList),
			IsExpanding:       isExpanding,
			ExpandErr:         expandErrstr,
			State:             string(volume.Status.State),
			VolumeName:        volume.Name,
			// CreatedAt:        volume.Status.KubernetesStatus.PVCName,
			// CreatedAt:        volume.Status.CreatedAt,
		}
		result[volume.Status.KubernetesStatus.PVCName+":"+volume.Status.KubernetesStatus.Namespace] = vs
	}

	self.JsonResponseWithoutError(http, result)
}

/*
*

	扩容卷
*/
func (self Longhorn) Attach(http *gin.Context) {
	// {"hostId":"server1","disableFrontend":true,"AttachedBy":"","attacherType":"","AttachmentID":"longhorn-ui"}
	type VolumeAttach struct {
		HostId          string `json:"hostId" binding:"required"`
		DisableFrontend bool   `json:"disableFrontend"`
		AttachedBy      string `json:"AttachedBy"`
		AttachmentID    string `json:"AttachmentID"`
		AttacherType    string `json:"attacherType"`
	}
	params := VolumeAttach{}
	if !self.Validate(http, &params) {
		return
	}
	if params.AttachmentID == "" {
		params.AttachmentID = "longhorn-ui"
	}
	volName := http.Param("volumeName")
	err := longhorn.LonghornVolumeAttach(volName, params.HostId, params.AttachmentID, params.AttachedBy, params.AttacherType)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonSuccessResponse(http)

}

func (self Longhorn) Detach(http *gin.Context) {
	//{forceDetach: true, attachmentID: "longhorn-ui", hostId: ""}
	type VolumeDetach struct {
		ForceDetach  bool   `json:"forceDetach"`
		AttachmentID string `json:"attachmentID"`
		HostId       string `json:"hostId"`
	}
	params := VolumeDetach{}
	if !self.Validate(http, &params) {
		return
	}
	if params.AttachmentID == "" {
		params.AttachmentID = "longhorn-ui"
	}
	volName := http.Param("volumeName")
	err := longhorn.LonghornVolumeDetach(volName, params.AttachmentID, params.ForceDetach)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponse(http, nil, nil, 200)
}
func (self Longhorn) CancelExpansion(http *gin.Context) {
	//{forceDetach: true, attachmentID: "longhorn-ui", hostId: ""}

	volName := http.Param("volumeName")
	err := longhorn.LonghornVolumeCancelExpansion(volName)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponse(http, nil, nil, 200)
}
