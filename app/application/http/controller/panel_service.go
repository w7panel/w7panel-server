package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PanelService struct{ controller.Abstract }

func (self PanelService) List(ctx *gin.Context) {
	if ctx.Param("namespace") == "" || ctx.Param("group") == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "缺少应用分组信息"})
		return
	}
	token := ctx.GetString("k8s_token")
	if token == "" {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "缺少 Kubernetes 凭据"})
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	services, err := appgroup.ListAppGroupServices(ctx.Request.Context(), sdk.ClientSet, ctx.Param("namespace"), ctx.Param("group"))
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusBadRequest)
		return
	}
	self.JsonResponseWithoutError(ctx, services)
}

func (self PanelService) Reconcile(ctx *gin.Context) {
	namespace, kind, name := ctx.Param("namespace"), ctx.Param("kind"), ctx.Param("name")
	if namespace == "" || name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "缺少工作负载信息"})
		return
	}
	token := ctx.GetString("k8s_token")
	if token == "" {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "缺少 Kubernetes 凭据"})
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	var workload metav1.Object
	var template corev1.PodTemplateSpec
	var ownerKind string
	switch kind {
	case "deployments":
		ownerKind = "Deployment"
		item, getErr := sdk.ClientSet.AppsV1().Deployments(namespace).Get(ctx.Request.Context(), name, metav1.GetOptions{})
		err = getErr
		if err == nil {
			workload, template = item, item.Spec.Template
		}
	case "statefulsets":
		ownerKind = "StatefulSet"
		item, getErr := sdk.ClientSet.AppsV1().StatefulSets(namespace).Get(ctx.Request.Context(), name, metav1.GetOptions{})
		err = getErr
		if err == nil {
			workload, template = item, item.Spec.Template
		}
	case "daemonsets":
		ownerKind = "DaemonSet"
		item, getErr := sdk.ClientSet.AppsV1().DaemonSets(namespace).Get(ctx.Request.Context(), name, metav1.GetOptions{})
		err = getErr
		if err == nil {
			workload, template = item, item.Spec.Template
		}
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "不支持的工作负载类型"})
		return
	}
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusBadRequest)
		return
	}
	service, err := appgroup.ReconcilePanelService(ctx.Request.Context(), sdk.ClientSet, workload, ownerKind, template)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusConflict)
		return
	}
	self.JsonResponseWithoutError(ctx, service)
}
