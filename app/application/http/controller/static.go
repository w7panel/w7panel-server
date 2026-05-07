package controller

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"k8s.io/apimachinery/pkg/api/errors"
)

type Static struct {
	controller.Abstract
}

func (self Static) StaticInfo(http *gin.Context) {
	identifie := http.Param("identifie")
	version := http.Query("version")
	releaseName := http.Query("releaseName")
	if strings.Contains(releaseName, "-root") {
		releaseName = strings.ReplaceAll(releaseName, "-root", "")
	}
	status := appgroup.DownStaticStatus(identifie, version, releaseName)
	self.JsonResponseWithoutError(http, gin.H{
		"status": status,
	})
}

func (self Static) Download(http *gin.Context) {
	name := http.Param("name")
	namespace := http.Param("namespace")
	token := http.MustGet("k8s_token").(string)

	rootSdk := k8s.NewK8sClient().Sdk
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	useSdk := sdk
	hasRoot := strings.Contains(name, "-root")
	if hasRoot {
		name = strings.ReplaceAll(name, "-root", "")
		useSdk = rootSdk
	}
	appgroupObj, err := appgroup.GetAppgroupUseSdk(name, namespace, useSdk)
	if err != nil {
		// 尝试从root集群获取
		if errors.IsNotFound(err) {
			group, err := appgroup.GetAppgroupUseSdk(name, namespace, rootSdk)
			if err != nil {
				self.JsonResponseWithServerError(http, err)
				return
			}
			appgroupObj = group
		} else {
			self.JsonResponseWithServerError(http, err)
			return
		}
	}
	appgroup.DownStatic(appgroupObj)

}
