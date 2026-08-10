package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/audit"
	"github.com/w7panel/w7panel/common/service/k8s"
	k8smetrics "github.com/w7panel/w7panel/common/service/k8s/metrics"
	"github.com/w7panel/w7panel/common/service/traffic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (self Metrics) TrafficHealth(ctx *gin.Context) {
	checkCtx, cancel := context.WithTimeout(ctx.Request.Context(), 5*time.Second)
	defer cancel()
	err := traffic.NewQueryClient().Health(checkCtx)
	hubbleAvailable := metricHasData(checkCtx, "hubble_flows_processed_total")
	higressAvailable := metricHasData(checkCtx, "envoy_cluster_upstream_rq_total")
	self.JsonResponseWithoutError(ctx, gin.H{
		"logs":           gin.H{"available": err == nil, "message": errorMessage(err)},
		"hubble":         gin.H{"available": hubbleAvailable},
		"higressMetrics": gin.H{"available": higressAvailable},
	})
}

func (self Metrics) TrafficSummary(ctx *gin.Context) {
	params, ok := parseTrafficParams(ctx)
	if !ok {
		return
	}
	rows, err := traffic.NewQueryClient().Summary(ctx.Request.Context(), params)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusInternalServerError)
		return
	}
	folded := traffic.FoldRows(rows)
	if len(folded) == 0 {
		self.JsonResponseWithoutError(ctx, gin.H{})
		return
	}
	self.JsonResponseWithoutError(ctx, folded[0])
}

func (self Metrics) TrafficPods(ctx *gin.Context) {
	params, ok := parseTrafficParams(ctx)
	if !ok {
		return
	}
	rows, err := traffic.NewQueryClient().Pods(ctx.Request.Context(), params)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusInternalServerError)
		return
	}
	rows = traffic.FoldRows(rows, "upstream_ip", "upstream_service", "upstream_namespace")
	traffic.SortRows(rows, params.Sort)
	resolvePodNames(ctx, params.Namespace, rows)
	rows = traffic.SearchRows(rows, params.Search, "pod_name", "upstream_ip", "upstream_service", "upstream_namespace")
	list, total := traffic.Paginate(rows, params.Page, params.PageSize)
	self.JsonResponseWithoutError(ctx, gin.H{"list": list, "total": total, "page": params.Page, "pageSize": params.PageSize})
}

func (self Metrics) TrafficDomains(ctx *gin.Context) {
	params, ok := parseTrafficParams(ctx)
	if !ok {
		return
	}
	rows, err := traffic.NewQueryClient().Domains(ctx.Request.Context(), params)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusInternalServerError)
		return
	}
	rows = traffic.FoldRows(rows, "authority")
	traffic.SortRows(rows, params.Sort)
	rows = traffic.SearchRows(rows, params.Search, "authority")
	list, total := traffic.Paginate(rows, params.Page, params.PageSize)
	self.JsonResponseWithoutError(ctx, gin.H{"list": list, "total": total, "page": params.Page, "pageSize": params.PageSize})
}

func (self Metrics) TrafficURLs(ctx *gin.Context) {
	params, ok := parseTrafficParams(ctx)
	if !ok {
		return
	}
	rows, err := traffic.NewQueryClient().URLs(ctx.Request.Context(), params)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusInternalServerError)
		return
	}
	rows = traffic.FoldRows(rows, "authority", "method", "path")
	traffic.SortRows(rows, params.Sort)
	list, total := traffic.Paginate(rows, params.Page, params.PageSize)
	self.JsonResponseWithoutError(ctx, gin.H{"list": list, "total": total, "page": params.Page, "pageSize": params.PageSize})
}

func (self Metrics) TrafficSeries(ctx *gin.Context) {
	params, ok := parseTrafficParams(ctx)
	if !ok {
		return
	}
	rows, err := traffic.NewQueryClient().Series(ctx.Request.Context(), params)
	if err != nil {
		self.JsonResponseWithError(ctx, err, http.StatusInternalServerError)
		return
	}
	self.JsonResponseWithoutError(ctx, rows)
}

func parseTrafficParams(ctx *gin.Context) (traffic.QueryParams, bool) {
	rangeValue, err := traffic.ParseTimeRange(ctx.Query("start"), ctx.Query("end"), time.Now())
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return traffic.QueryParams{}, false
	}
	current := audit.CurrentUser(ctx)
	namespace := strings.TrimSpace(ctx.DefaultQuery("namespace", current.Tenant))
	if namespace == "" {
		namespace = "default"
	}
	if !current.IsAdmin {
		// Traffic metrics are collected in the host cluster, where each normal
		// user's virtual cluster resides in its own k3k-{username} namespace.
		// Never trust a caller-provided namespace for a normal user.
		namespace = "k3k-" + strings.TrimSpace(current.Username)
	}
	if namespace == "" {
		namespace = "default"
	}
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("pageSize", "20"))
	params := traffic.QueryParams{Namespace: namespace, Domain: strings.TrimSpace(ctx.Query("domain")), UpstreamIP: strings.TrimSpace(ctx.Query("upstreamIp")), Method: strings.TrimSpace(ctx.Query("method")), Status: strings.TrimSpace(ctx.Query("status")), Keyword: strings.TrimSpace(ctx.Query("keyword")), Search: strings.TrimSpace(ctx.Query("search")), Sort: strings.TrimSpace(ctx.Query("sort")), Step: strings.TrimSpace(ctx.Query("step")), Page: page, PageSize: pageSize, Range: rangeValue}
	traffic.NormalizeParams(&params)
	return params, true
}

func resolvePodNames(ctx *gin.Context, namespace string, rows []map[string]any) {
	sdk := k8s.NewK8sClient().Sdk
	if namespace == "*" {
		namespace = metav1.NamespaceAll
	}
	pods, err := sdk.ClientSet.CoreV1().Pods(namespace).List(ctx.Request.Context(), metav1.ListOptions{})
	if err != nil {
		return
	}
	byIP := map[string][2]string{}
	for _, pod := range pods.Items {
		if pod.Status.PodIP != "" {
			byIP[pod.Status.PodIP] = [2]string{pod.Name, pod.Namespace}
		}
	}
	for _, row := range rows {
		ip, _ := row["upstream_ip"].(string)
		if pod, exists := byIP[ip]; exists {
			row["pod_name"] = pod[0]
			row["namespace"] = pod[1]
		} else {
			row["pod_name"] = "已终止 Pod"
			row["namespace"] = row["upstream_namespace"]
		}
	}
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func metricHasData(ctx context.Context, query string) bool {
	body, err := k8smetrics.Query(ctx, k8s.NewK8sClient().Sdk, map[string]string{"query": query})
	if err != nil {
		return false
	}
	var response struct {
		Data struct {
			Result []any `json:"result"`
		} `json:"data"`
	}
	return json.Unmarshal(body, &response) == nil && len(response.Data.Result) > 0
}
