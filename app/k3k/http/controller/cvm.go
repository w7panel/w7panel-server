package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Cvm struct {
	controller.Abstract
}

/*
*
 */
func (self Cvm) Info(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	k8sToken := k8s.NewK8sToken(token)
	name := http.Param("name")
	namespace := http.Param("namespace")

	rootSdk := k8s.NewK8sClient()
	sigClient, err := rootSdk.ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if k8sToken.IsK3kCluster() {
		namespace = k8sToken.GetNamespace()
	}
	cvm := &v1alpha1.Cvm{}
	err = sigClient.Get(http, types.NamespacedName{Namespace: namespace, Name: name}, cvm)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	cvm.ComputeStatus()
	self.JsonResponseWithoutError(http, cvm)

}
func (self Cvm) List(http *gin.Context) {
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
	list := &v1alpha1.CvmList{}
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
