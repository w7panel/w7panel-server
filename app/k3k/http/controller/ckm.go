package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	v1alpha1 "github.com/w7panel/w7panel/common/service/k8s/ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Ckm struct{ controller.Abstract }

func (self Ckm) Info(http *gin.Context) {
	c, err := k3k.TokenToCkm(http, http.MustGet("k8s_token").(string), http.Param("namespace"), http.Param("name"))
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	c.ComputeStatus()
	self.JsonResponseWithoutError(http, c)
}

func (self Ckm) List(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	namespace := http.Query("namespace")
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	if !user.IsFounder() {
		namespace = k8s.NewK8sToken(token).GetNamespace()
	}
	sigClient, err := k8s.NewK8sClient().ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	list := &v1alpha1.CkmList{}
	options := &client.ListOptions{}
	if namespace != "" {
		options.ApplyOptions([]client.ListOption{client.InNamespace(namespace)})
	}
	if err = sigClient.List(http, list, options); err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	for i := range list.Items {
		list.Items[i].ComputeStatus()
	}
	self.JsonResponseWithoutError(http, list)
}
