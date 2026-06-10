package controller

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	"github.com/w7panel/w7panel/common/service/k8s/metrics"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Metrics struct {
	controller.Abstract
}

type MetricsInstall struct {
	Installed bool   `json:"installed"`
	BaseUrl   string `json:"baseUrl"`
	Namespace string `json:"namespace"`
}

func (self Metrics) VmOperatorInstalled(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	k8stoken := k8s.NewK8sToken(token)
	rootSdk := k8s.NewK8sClient() //不能.Sdk
	namespace := "w7-system"
	releaseName := "vm-operator"
	isVirtual := k8stoken.IsVirtual()
	sdk := rootSdk.Sdk
	result := &MetricsInstall{
		BaseUrl:   "/k8s-proxy/v1/namespaces/w7-system/services/vmsingle-vm-operator-k8s-offline-metrics-single:8429/proxy/",
		Installed: false,
		Namespace: namespace,
	}
	if isVirtual {
		result.BaseUrl = "/api/v1/namespaces/default/services/vmsingle-w7panel-metrics-k8s-offline-metrics-single:8429/proxy/"
		result.Namespace = "default"
		releaseName = "w7panel-metrics"
		client, err := rootSdk.Channel(token)
		if err != nil {
			slog.Error("channel error", "error", err)
			result.Installed = false
			self.JsonResponseWithoutError(http, result)
			return
		}
		sdk = client
	}
	helmApi := k8s.NewHelm(sdk)
	_, err := helmApi.Info(releaseName, result.Namespace)

	if err != nil {
		result.Installed = false
		self.JsonResponseWithoutError(http, result)
		return
	}
	result.Installed = true
	self.JsonResponseWithoutError(http, result)
}

func (self Metrics) MetricsState(http *gin.Context) {

	type MetricsState struct {
		CanShowClusterMetrics         bool `json:"canShowClusterMetrics"`
		CanShowNodeMetrics            bool `json:"canShowNodeMetrics"`
		CanShowPodMetrics             bool `json:"canShowPodMetrics"`
		NeedInstallMetricsInDashboard bool `json:"needInstallMetricsInDashboard"`
		NeedInstallMetricsInApp       bool `json:"needInstallMetricsInApp"`
	}
	token := http.MustGet("k8s_token").(string)
	k8stoken := k8s.NewK8sToken(token)
	rootSdk := k8s.NewK8sClient() //不能.Sdk

	state := &MetricsState{
		CanShowClusterMetrics:         false,
		CanShowNodeMetrics:            false,
		CanShowPodMetrics:             false,
		NeedInstallMetricsInDashboard: false,
		NeedInstallMetricsInApp:       false,
		// NeedInstallPodMetrics: false,
	}
	releaseName := "w7panel-metrics"
	sdk := rootSdk.Sdk
	helmApi := k8s.NewHelm(sdk)
	_, err := helmApi.Info(releaseName, "default")
	rootInstalled := err == nil
	state.CanShowClusterMetrics = rootInstalled
	if k8stoken.IsK3kCluster() {
		childSdk, err := k8s.NewK8sClient().Channel(token)
		if err != nil {
			slog.Error("channel error", "error", err)
			self.JsonResponseWithoutError(http, state)
			return
		}
		childHelmApi := k8s.NewHelm(childSdk)
		_, childerr := childHelmApi.Info(releaseName, "default")
		if childerr == nil {
			state.CanShowPodMetrics = true
		} else {
			state.NeedInstallMetricsInApp = false
		}
	} else {
		state.CanShowNodeMetrics = rootInstalled
		state.CanShowPodMetrics = rootInstalled
		state.NeedInstallMetricsInApp = !rootInstalled
		state.NeedInstallMetricsInDashboard = !rootInstalled
	}
	// childSdk, err := k8s.NewK8sClient().Channel(token)
	// if err != nil {
	// 	slog.Error("channel error", "error", err)
	// 	self.JsonResponseWithoutError(http, state)
	// 	return
	// }

	self.JsonResponseWithoutError(http, state)

}

func (self Metrics) Usage(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	uage := metrics.NewK3kUsage(k8s.NewK8sClient().Sdk)
	cpu, memory, cputotal, memorytotal, err := uage.GetResourceUsage(user)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	response := gin.H{
		"cpu": gin.H{
			"usage": cpu.MilliValue(),
			"total": cputotal.MilliValue(),
		},
		"memory": gin.H{
			"usage": memory.Value(),
			"total": memorytotal.Value(),
		},
	}
	self.JsonResponseWithoutError(http, response)
}

func (self Metrics) UsageDisk(http *gin.Context) {
	token := http.MustGet("k8s_token").(string)
	user, err := k3k.TokenToK3kUser(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	uage := metrics.NewK3kUsage(k8s.NewK8sClient().Sdk)
	usage, total, err := uage.GetResourceDiskUsage(user)
	if err != nil {
		response := gin.H{
			"disk": gin.H{
				"usage": usage,
				"total": total,
			},
		}
		self.JsonResponseWithoutError(http, response)
		return
	}
	response := gin.H{
		"disk": gin.H{
			"usage": usage,
			"total": total,
		},
	}
	self.JsonResponseWithoutError(http, response)
}
