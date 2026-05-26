package controller

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	v1alpha1 "github.com/w7panel/w7panel-ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Ckm struct {
	controller.Abstract
}

/*
*
 */
func (self Ckm) Info(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	namespace := http.Param("namespace")

	cvm, err := k3k.TokenToCkm(http, token, namespace, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	cvm.ComputeStatus()
	self.JsonResponseWithoutError(http, cvm)

}
func (self Ckm) List(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	ns := http.Query("namespace")
	rootSdk := k8s.NewK8sClient()
	sigClient, err := rootSdk.ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if !user.IsFounder() {
		ns = k8sToken.GetNamespace()
	}
	list := &v1alpha1.CkmList{}
	options := &client.ListOptions{}
	if ns != "" {
		options.ApplyOptions([]client.ListOption{
			client.InNamespace(ns),
		})
	}
	err = sigClient.List(http, list, options)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	for _, cvm := range list.Items {
		cvm.ComputeStatus()
	}
	self.JsonResponseWithoutError(http, list)
}

// 救援模式
func (self Ckm) RescueToggle(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	name := http.Param("name")
	namespace := http.Param("namespace")

	rootSdk := k8s.NewK8sClient()
	sigClient, err := rootSdk.ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	cvm, err := k3k.TokenToCkm(http, token, namespace, name)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	job := types.ToK3kWeihJob(cvm)
	err = sigClient.Delete(http, job)
	if err != nil {
		slog.Warn("delete job err", "err", err)
	}
	cvm.RescueToggle()
	err = sigClient.Update(http, cvm)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, cvm)
}
