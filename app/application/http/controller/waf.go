package controller

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/waf"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
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

func wafConfig(plugin *unstructured.Unstructured) (waf.Policy, map[string]interface{}, error) {
	config, _, err := unstructured.NestedMap(plugin.Object, "spec", "defaultConfig")
	if err != nil {
		return waf.Policy{}, nil, err
	}
	global := policyFrom(config["w7Policy"])
	if global.UseCRS == nil {
		useCRS, _ := config["useCRS"].(bool)
		global.UseCRS = &useCRS
	}
	return global, config, nil
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
	global, _, err := wafConfig(plugin)
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
	self.JsonResponseWithoutError(ctx, gin.H{"global": global, "overrides": overrides, "catalog": waf.SystemCatalog})
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
	global, defaultConfig, err := wafConfig(plugin)
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
		global = request.Policy
		if global.UseCRS == nil {
			useCRS := defaultConfig["useCRS"] == true
			global.UseCRS = &useCRS
		}
		compiled, err := waf.Compile(global, nil)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()})
			return
		}
		for key, value := range compiled {
			defaultConfig[key] = value
		}
		defaultConfig["w7Policy"] = policyMap(global)
		delete(defaultConfig, "w7LegacyRules")
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
			known := make(map[int]bool, len(global.CustomRules))
			for _, custom := range global.CustomRules {
				known[custom.ID] = true
			}
			kept := overrides.DisabledGlobalIDs[:0]
			for _, id := range overrides.DisabledGlobalIDs {
				if known[id] {
					kept = append(kept, id)
				}
			}
			overrides.DisabledGlobalIDs = kept
			compiled, err := waf.Compile(global, &overrides)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"msg": fmt.Sprintf("域名规则 %d: %v", index, err)})
				return
			}
			for key, value := range compiled {
				config[key] = value
			}
			config["w7Overrides"] = policyMap(overrides)
		}
		if err := unstructured.SetNestedMap(plugin.Object, jsonMap(defaultConfig), "spec", "defaultConfig"); err != nil {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
	} else {
		compiled, err := waf.Compile(global, &request.Policy)
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
