package controller

import (
	"encoding/json"

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
func (self Ckm) IdcResource(http *gin.Context) {
	sdk := k8s.NewK8sClient()
	client, err := sdk.ToSigClient()
	if err != nil {
		self.JsonSuccessResponse(http)
		return
	}
	list := &v1alpha1.CostList{}
	err = client.List(http, list)
	if err != nil {
		self.JsonResponseWithoutError(http, list)
		return
	}
	result := types.Params{}
	for _, v := range list.Items {
		if (v.Labels != nil) && (v.Labels["w7.cc/showInShop"] != "true") {
			continue
		}
		if v.Annotations == nil {
			continue
		}
		items := v.Annotations["w7.cc/package-items"]
		if items == "" {
			continue
		}
		params := types.Params{}
		err := json.Unmarshal([]byte(items), &params)
		if err != nil {
			continue
		}
		result = append(result, params...)
	}

	self.JsonResponseWithoutError(http, result)

}
