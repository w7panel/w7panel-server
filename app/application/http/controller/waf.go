package controller

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/waf"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type WAF struct{ controller.Abstract }

var wafResource = schema.GroupVersionResource{Group: "extensions.higress.io", Version: "v1alpha1", Resource: "wasmplugins"}

const wafName = "w7panel-pluginwaf"
const wafNamespace = "higress-system"

type wafRequest struct {
	Scope       string     `json:"scope" binding:"required"`
	Namespace   string     `json:"namespace"`
	IngressName string     `json:"ingressName"`
	Policy      waf.Policy `json:"policy"`
}

func wafClient(ctx *gin.Context) (*k8s.Sdk, bool) {
	token := ctx.GetString("k8s_token")
	if token == "" {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "缺少 Kubernetes 身份"})
		return nil, false
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法验证 Kubernetes 权限"})
		return nil, false
	}
	return sdk, true
}

func wafScope(ctx *gin.Context, sdk *k8s.Sdk, scope, namespace, ingressName string) (string, bool) {
	if scope == "global" {
		return "", true
	}
	if scope != "rule" || namespace == "" || ingressName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": "缺少域名规则上下文"})
		return "", false
	}
	ingress, err := sdk.ClientSet.NetworkingV1().Ingresses(namespace).Get(ctx.Request.Context(), ingressName, metav1.GetOptions{})
	if err != nil || len(ingress.Spec.Rules) == 0 || ingress.Spec.Rules[0].Host == "" {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法读取当前域名"})
		return "", false
	}
	return ingress.Spec.Rules[0].Host, true
}

func policyFrom(value interface{}) waf.Policy {
	var result waf.Policy
	data, _ := json.Marshal(value)
	_ = json.Unmarshal(data, &result)
	if result.Categories == nil {
		result.Categories = map[string]bool{}
	}
	if result.CustomRules == nil {
		result.CustomRules = []waf.Rule{}
	}
	return result
}

func policyMap(value waf.Policy) map[string]interface{} {
	data, _ := json.Marshal(value)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

func jsonMap(value map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(value)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

func jsonSlice(value []interface{}) []interface{} {
	data, _ := json.Marshal(value)
	var result []interface{}
	_ = json.Unmarshal(data, &result)
	return result
}

func stringSlice(value interface{}) []string {
	items, _ := value.([]interface{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func wafRules(plugin *unstructured.Unstructured) ([]interface{}, error) {
	rules, _, err := unstructured.NestedSlice(plugin.Object, "spec", "matchRules")
	return rules, err
}

func findWAFRule(rules []interface{}, target string) int {
	for index, item := range rules {
		rule, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		for _, name := range stringSlice(rule["ingress"]) {
			if name == target {
				return index
			}
		}
	}
	return -1
}

func wafConfig(plugin *unstructured.Unstructured) (waf.Policy, []string, map[string]interface{}, error) {
	config, _, err := unstructured.NestedMap(plugin.Object, "spec", "defaultConfig")
	if err != nil {
		return waf.Policy{}, nil, nil, err
	}
	global := policyFrom(config["w7Policy"])
	if global.UseCRS == nil {
		useCRS, _ := config["useCRS"].(bool)
		global.UseCRS = &useCRS
	}
	legacy := stringSlice(config["w7LegacyRules"])
	if config["w7Policy"] == nil {
		legacy = stringSlice(config["secRules"])
	}
	return global, legacy, config, nil
}

func (self WAF) Config(ctx *gin.Context) {
	sdk, ok := wafClient(ctx)
	if !ok {
		return
	}
	scope, namespace, ingressName := ctx.Query("scope"), ctx.Query("namespace"), ctx.Query("ingressName")
	if _, ok := wafScope(ctx, sdk, scope, namespace, ingressName); !ok {
		return
	}
	plugin, err := sdk.DynamicClient().Resource(wafResource).Namespace(wafNamespace).Get(ctx.Request.Context(), wafName, metav1.GetOptions{})
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法读取 WAF 插件配置"})
		return
	}
	global, legacy, _, err := wafConfig(plugin)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	overrides := waf.Policy{Categories: map[string]bool{}, CustomRules: []waf.Rule{}}
	if scope == "rule" {
		rules, err := wafRules(plugin)
		if err != nil {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
		if index := findWAFRule(rules, namespace+"/"+ingressName); index >= 0 {
			rule := rules[index].(map[string]interface{})
			config, _ := rule["config"].(map[string]interface{})
			overrides = policyFrom(config["w7Overrides"])
			if config["w7Overrides"] == nil {
				if useCRS, ok := config["useCRS"].(bool); ok && useCRS != *global.UseCRS {
					overrides.UseCRS = &useCRS
				}
			}
		}
	}
	global.LegacyRules = legacy
	self.JsonResponseWithoutError(ctx, gin.H{"global": global, "overrides": overrides})
}

func (self WAF) SaveConfig(ctx *gin.Context) {
	var request wafRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
		return
	}
	sdk, ok := wafClient(ctx)
	if !ok {
		return
	}
	if _, ok := wafScope(ctx, sdk, request.Scope, request.Namespace, request.IngressName); !ok {
		return
	}
	resource := sdk.DynamicClient().Resource(wafResource).Namespace(wafNamespace)
	plugin, err := resource.Get(ctx.Request.Context(), wafName, metav1.GetOptions{})
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法修改 WAF 插件配置"})
		return
	}
	global, legacy, defaultConfig, err := wafConfig(plugin)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	rules, err := wafRules(plugin)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	if request.Scope == "global" {
		if request.Policy.LegacyRules != nil {
			available := make(map[string]int)
			for _, rule := range legacy {
				available[rule]++
			}
			for _, rule := range request.Policy.LegacyRules {
				available[rule]--
				if available[rule] < 0 {
					ctx.JSON(http.StatusBadRequest, gin.H{"msg": "不能新增旧版规则指令"})
					return
				}
			}
			legacy = request.Policy.LegacyRules
		}
		global = request.Policy
		if global.UseCRS == nil {
			useCRS := defaultConfig["useCRS"] == true
			global.UseCRS = &useCRS
		}
		global.LegacyRules = nil
		compiled, err := waf.Compile(global, nil, legacy)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
			return
		}
		for key, value := range compiled {
			defaultConfig[key] = value
		}
		defaultConfig["w7Policy"] = policyMap(global)
		defaultConfig["w7LegacyRules"] = legacy
		for index, item := range rules {
			rule, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			config, ok := rule["config"].(map[string]interface{})
			if !ok || config["w7Overrides"] == nil {
				continue
			}
			overrides := policyFrom(config["w7Overrides"])
			compiled, err := waf.Compile(global, &overrides, legacy)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"msg": fmt.Sprintf("域名规则 %d: %v", index, err)})
				return
			}
			for key, value := range compiled {
				config[key] = value
			}
		}
		if err := unstructured.SetNestedMap(plugin.Object, jsonMap(defaultConfig), "spec", "defaultConfig"); err != nil {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
	} else {
		compiled, err := waf.Compile(global, &request.Policy, legacy)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
			return
		}
		target := request.Namespace + "/" + request.IngressName
		index := findWAFRule(rules, target)
		if index < 0 {
			rules = append(rules, map[string]interface{}{"ingress": []interface{}{target}, "config": map[string]interface{}{}, "configDisable": false})
			index = len(rules) - 1
		} else {
			rule := rules[index].(map[string]interface{})
			remaining := make([]interface{}, 0)
			for _, name := range stringSlice(rule["ingress"]) {
				if name != target {
					remaining = append(remaining, name)
				}
			}
			hasOtherTargets := len(remaining) > 0
			for key, value := range rule {
				if key != "ingress" && key != "config" && key != "configDisable" {
					if items, ok := value.([]interface{}); ok && len(items) > 0 {
						hasOtherTargets = true
					}
				}
			}
			if hasOtherTargets {
				isolated := jsonMap(rule)
				isolated["ingress"] = []interface{}{target}
				for key, value := range isolated {
					if key != "ingress" && key != "config" && key != "configDisable" {
						if _, ok := value.([]interface{}); ok {
							delete(isolated, key)
						}
					}
				}
				if len(remaining) > 0 {
					rule["ingress"] = remaining
				} else {
					delete(rule, "ingress")
				}
				rules = append(rules, isolated)
				index = len(rules) - 1
			}
		}
		rule := rules[index].(map[string]interface{})
		config, _ := rule["config"].(map[string]interface{})
		if config == nil {
			config = map[string]interface{}{}
		}
		for key, value := range compiled {
			config[key] = value
		}
		config["w7Overrides"] = policyMap(request.Policy)
		rule["config"] = config
		rule["configDisable"] = false
	}
	if len(rules) > 0 {
		if err := unstructured.SetNestedSlice(plugin.Object, jsonSlice(rules), "spec", "matchRules"); err != nil {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
	}
	if _, err := resource.Update(ctx.Request.Context(), plugin, metav1.UpdateOptions{}); err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	self.JsonResponseWithoutError(ctx, gin.H{"saved": true})
}

type wafLog struct {
	Key      string `json:"key"`
	Time     string `json:"time"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	RuleID   string `json:"ruleId"`
	Action   string `json:"action"`
	ClientIP string `json:"clientIp"`
}

type wafAccessLog struct {
	Authority               string `json:"authority"`
	Path                    string `json:"path"`
	ResponseCodeDetails     string `json:"response_code_details"`
	RequestID               string `json:"request_id"`
	StartTime               string `json:"start_time"`
	XForwardedFor           string `json:"x_forwarded_for"`
	DownstreamRemoteAddress string `json:"downstream_remote_address"`
}

var wafRequestID = regexp.MustCompile(`\[[0-9a-fA-F]{8}-[0-9a-fA-F-]{27,}\]`)
var wafRuleID = regexp.MustCompile(`\[id "([0-9]+)"\]`)

func parseWAFLogs(data []byte, domain string) []wafLog {
	result := make([]wafLog, 0)
	matchedIDs := make(map[string]string)
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if !bytes.Contains(line, []byte("w7panel-pluginwaf")) {
			continue
		}
		request := wafRequestID.Find(line)
		match := wafRuleID.FindSubmatch(line)
		if len(request) > 2 && len(match) == 2 {
			matchedIDs[string(request[1:len(request)-1])] = string(match[1])
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var row wafAccessLog
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.ResponseCodeDetails != "waf" {
			continue
		}
		host := row.Authority
		if name, _, err := net.SplitHostPort(host); err == nil {
			host = name
		}
		if domain != "" && !strings.EqualFold(host, domain) {
			continue
		}
		path := strings.SplitN(row.Path, "?", 2)[0]
		if parsed, err := url.ParseRequestURI(path); err == nil {
			path = parsed.Path
		}
		ip := strings.TrimSpace(strings.Split(row.XForwardedFor, ",")[0])
		if ip == "" || ip == "-" {
			ip, _, _ = net.SplitHostPort(row.DownstreamRemoteAddress)
		}
		result = append(result, wafLog{Key: row.RequestID, Time: row.StartTime, Domain: host, Path: path, RuleID: matchedIDs[row.RequestID], Action: "拦截", ClientIP: ip})
	}
	return result
}

func (self WAF) Logs(ctx *gin.Context) {
	sdk, ok := wafClient(ctx)
	if !ok {
		return
	}
	scope := ctx.Query("scope")
	domain, ok := wafScope(ctx, sdk, scope, ctx.Query("namespace"), ctx.Query("ingressName"))
	if !ok {
		return
	}
	if scope == "global" {
		// A global view needs the user's own permission to read gateway logs.
		if _, err := sdk.ClientSet.CoreV1().Pods(wafNamespace).List(ctx.Request.Context(), metav1.ListOptions{LabelSelector: "app=higress-gateway", Limit: 1}); err != nil {
			ctx.JSON(http.StatusForbidden, gin.H{"msg": "没有网关日志读取权限"})
			return
		}
	}
	pods, err := k8s.NewK8sClient().Sdk.ClientSet.CoreV1().Pods(wafNamespace).List(ctx.Request.Context(), metav1.ListOptions{LabelSelector: "app=higress-gateway"})
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	result := make([]wafLog, 0)
	tail := int64(5000)
	since := int64(86400)
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		data, err := k8s.NewK8sClient().Sdk.ClientSet.CoreV1().Pods(wafNamespace).GetLogs(pod.Name, &corev1.PodLogOptions{TailLines: &tail, SinceSeconds: &since}).DoRaw(ctx.Request.Context())
		if err != nil {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
		result = append(result, parseWAFLogs(data, domain)...)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Time > result[j].Time })
	if len(result) > 100 {
		result = result[:100]
	}
	self.JsonResponseWithoutError(ctx, result)
}
