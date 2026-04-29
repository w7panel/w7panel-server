package controller

import (
	"log/slog"

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
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if !user.IsFounder() {
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

// 救援模式
func (self Cvm) RescueToggle(http *gin.Context) {
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
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if user.SupportCvm() {
		namespace = k8sToken.GetNamespace()
	}
	cvm := &v1alpha1.Cvm{}
	err = sigClient.Get(http, types.NamespacedName{Namespace: namespace, Name: name}, cvm)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	cvm.RescueToggle()
	err = sigClient.Update(http, cvm)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, cvm)
}

func (self Cvm) CheckResource(http *gin.Context) {

	type Result struct {
		Pass bool `json:"pass"`
	}
	result := Result{
		Pass: false,
	}
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
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if user.SupportCvm() {
		namespace = k8sToken.GetNamespace()
	}
	cvm := &v1alpha1.Cvm{}
	err = sigClient.Get(http, types.NamespacedName{Namespace: namespace, Name: name}, cvm)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	err = k3k.TryCheckOverSellingResource(rootSdk.Sdk, cvm)
	if err != nil {
		slog.Error("集群资源不足", "error", err)
		self.JsonResponseWithoutError(http, result)
		return
	}
	result.Pass = true // 集群资源充足
	self.JsonResponseWithoutError(http, result)
	return

}

// 同步用户的集群
func (self Cvm) Sync(http *gin.Context) {

	token := http.MustGet("k8s_token").(string)
	rootSdk := k8s.NewK8sClient()

	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	err = k3k.SyncUserToCvm(http, user, rootSdk.Sdk)
	if err != nil {
		slog.Error("同步用户集群失败", "error", err)
		self.JsonSuccessResponse(http)
		return
	}
	self.JsonSuccessResponse(http)

}
