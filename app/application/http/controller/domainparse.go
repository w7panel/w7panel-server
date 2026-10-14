package controller

import (
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type DomainParse struct {
	controller.Abstract
}

// DomainParse returns the domain parsing configuration used during application installation.
func (self DomainParse) Get(http *gin.Context) {
	client := k8s.NewK8sClient()

	config, err := client.GetConfigCRD(http.Request.Context(), k8s.DomainParseConfigGVR, k8s.DomainParseConfigName)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, config)
}
