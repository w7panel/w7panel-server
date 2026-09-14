package controller

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
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

var (
	queryMetricsRange = metrics.QueryRange
	resolveMetricsSDK = metricsSDKForToken
)

func metricsSDKForToken(token string, forceLocal bool) (*k8s.Sdk, error) {
	return k8s.NewK8sClient().Channel(token)
}

func (self Metrics) QueryRange(httpContext *gin.Context) {
	query := strings.TrimSpace(httpContext.Query("query"))
	if query == "" {
		httpContext.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "msg": "query不能为空"})
		return
	}

	params := map[string]string{"query": query}
	for _, name := range []string{"start", "end", "step"} {
		if value := strings.TrimSpace(httpContext.Query(name)); value != "" {
			params[name] = value
		}
	}

	local := httpContext.Query("local")
	forceLocal := local == "1" || strings.EqualFold(local, "true")
	sdk, err := resolveMetricsSDK(httpContext.MustGet("k8s_token").(string), forceLocal)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	ctx, cancel := context.WithTimeout(httpContext.Request.Context(), 30*time.Second)
	defer cancel()
	data, err := queryMetricsRange(ctx, sdk, params)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	httpContext.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

func (self Metrics) VmOperatorInstalled(http *gin.Context) {
	rootSdk := k8s.NewK8sClient() //不能.Sdk
	namespace := "w7-system"
	releaseName := "vm-operator"
	sdk := rootSdk.Sdk
	result := &MetricsInstall{
		BaseUrl:   "/k8s-proxy/v1/namespaces/w7-system/services/vmsingle-vm-operator-k8s-offline-metrics-single:8429/proxy/",
		Installed: false,
		Namespace: namespace,
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
	state.CanShowNodeMetrics = rootInstalled
	state.CanShowPodMetrics = rootInstalled
	state.NeedInstallMetricsInApp = !rootInstalled
	state.NeedInstallMetricsInDashboard = !rootInstalled

	self.JsonResponseWithoutError(http, state)

}

func (self Metrics) Usage(http *gin.Context) {
	usage := metrics.NewLocalUsage(k8s.NewK8sClient().Sdk)
	cpu, memory, cputotal, memorytotal, err := usage.GetResourceUsage()
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
	usageApi := metrics.NewLocalUsage(k8s.NewK8sClient().Sdk)
	usage, total, err := usageApi.GetResourceDiskUsage()
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
