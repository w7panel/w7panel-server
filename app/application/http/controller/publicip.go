package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
)

const publicIPLabel = "w7.public-ip"

func (self Nodes) LocalPublicIP(ctx *gin.Context) {
	ip, err := helper.MyIp()
	if err != nil || strings.Contains(ip, ":") || net.ParseIP(strings.TrimSpace(ip)).To4() == nil {
		ctx.JSON(stdhttp.StatusBadGateway, gin.H{"error": "无法获取有效的 IPv4 地址"})
		return
	}
	ctx.JSON(stdhttp.StatusOK, gin.H{"ip": strings.TrimSpace(ip)})
}

func (self Nodes) DetectPublicIP(ctx *gin.Context) {
	if helper.IsChildAgent() {
		ctx.JSON(stdhttp.StatusForbidden, gin.H{"error": "子集群不支持修改公网 IP"})
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(ctx.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	node, err := sdk.ClientSet.CoreV1().Nodes().Get(ctx.Request.Context(), ctx.Param("name"), metav1.GetOptions{})
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	var hostIP string
	for _, address := range node.Status.Addresses {
		if address.Type == corev1.NodeInternalIP {
			hostIP = address.Address
			break
		}
	}
	if net.ParseIP(hostIP) == nil {
		self.JsonResponseWithServerError(ctx, errors.New("节点缺少内网 IP"))
		return
	}
	agent, err := sdk.GetDaemonsetAgentPod(sdk.GetNamespace(), hostIP)
	if err != nil || agent.Status.PodIP == "" {
		self.JsonResponseWithServerError(ctx, errors.New("目标节点 agent 不可用"))
		return
	}
	request, err := stdhttp.NewRequestWithContext(ctx.Request.Context(), stdhttp.MethodGet, "http://"+net.JoinHostPort(agent.Status.PodIP, "8000")+"/internal/public-ip", nil)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	request.Header.Set("Authorization", "Bearer "+ctx.MustGet("k8s_token").(string))
	response, err := (&stdhttp.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	defer response.Body.Close()
	var result struct {
		IP string `json:"ip"`
	}
	if response.StatusCode != stdhttp.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil || strings.Contains(result.IP, ":") || net.ParseIP(result.IP).To4() == nil {
		self.JsonResponseWithServerError(ctx, errors.New("目标节点未返回有效的 IPv4 地址"))
		return
	}
	self.JsonResponseWithoutError(ctx, gin.H{"ip": result.IP})
}

func isServerNode(node *corev1.Node) bool {
	_, controlPlane := node.Labels["node-role.kubernetes.io/control-plane"]
	_, master := node.Labels["node-role.kubernetes.io/master"]
	return controlPlane || master
}

func publicSANs(nodes []corev1.Node) []string {
	addresses := []string{}
	seen := map[string]bool{}
	for _, node := range nodes {
		ip := node.Labels[publicIPLabel]
		if isServerNode(&node) && !strings.Contains(ip, ":") && net.ParseIP(ip).To4() != nil && !seen[ip] {
			addresses = append(addresses, ip)
			seen[ip] = true
		}
	}
	return addresses
}

func (self Nodes) SavePublicIP(ctx *gin.Context) {
	if helper.IsChildAgent() {
		ctx.JSON(stdhttp.StatusForbidden, gin.H{"error": "子集群不支持修改公网 IP"})
		return
	}
	var input struct {
		IP *string `json:"ip"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		self.JsonResponseWithError(ctx, err, stdhttp.StatusBadRequest)
		return
	}
	if input.IP == nil {
		self.JsonResponseWithError(ctx, errors.New("缺少 ip 字段"), stdhttp.StatusBadRequest)
		return
	}
	ip := strings.TrimSpace(*input.IP)
	if ip != "" && (strings.Contains(ip, ":") || net.ParseIP(ip).To4() == nil) {
		self.JsonResponseWithError(ctx, errors.New("请输入有效的 IPv4 地址"), stdhttp.StatusBadRequest)
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(ctx.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	nodes := sdk.ClientSet.CoreV1().Nodes()
	name := ctx.Param("name")
	var oldIP string
	var server bool
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := nodes.Get(ctx.Request.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		oldIP, server = node.Labels[publicIPLabel], isServerNode(node)
		if oldIP == ip {
			return nil
		}
		if node.Labels == nil {
			node.Labels = map[string]string{}
		}
		if ip == "" {
			delete(node.Labels, publicIPLabel)
		} else {
			node.Labels[publicIPLabel] = ip
		}
		_, err = nodes.Update(ctx.Request.Context(), node, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	if !server {
		self.JsonResponseWithoutError(ctx, gin.H{"ip": ip, "restartTriggered": false})
		return
	}
	configChanged := false
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		all, err := nodes.List(ctx.Request.Context(), metav1.ListOptions{})
		if err != nil {
			return err
		}
		config, err := sdk.DynamicClient().Resource(k8s.K3sConfigGVR).Get(ctx.Request.Context(), k8s.K3sConfigName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		data := k8s.ConfigCRDData(config)
		public := publicSANs(all.Items)
		publicValue := strings.Join(public, ",")
		customValue := filterPublicSANs(data["k3s.tls-san"], oldIP, public)
		disabledValue := filterPublicSANs(data["k3s.tls-san.disabled"], oldIP, public)
		if data["k3s.public-tls-san"] == publicValue && data["k3s.tls-san"] == customValue && data["k3s.tls-san.disabled"] == disabledValue {
			configChanged = false
			return nil
		}
		data["k3s.public-tls-san"] = publicValue
		data["k3s.tls-san"] = customValue
		data["k3s.tls-san.disabled"] = disabledValue
		configChanged = true
		if err = unstructured.SetNestedStringMap(config.Object, data, "spec", "data"); err != nil {
			return err
		}
		labels := config.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels["data-hash"] = time.Now().UTC().Format("20060102150405.000000000")
		config.SetLabels(labels)
		_, err = sdk.DynamicClient().Resource(k8s.K3sConfigGVR).Update(ctx.Request.Context(), config, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		if oldIP == ip {
			self.JsonResponseWithServerError(ctx, err)
			return
		}
		rollbackErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, getErr := nodes.Get(ctx.Request.Context(), name, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			if node.Labels[publicIPLabel] != ip {
				return errors.New("节点公网 IP 已被其他操作修改")
			}
			if oldIP == "" {
				delete(node.Labels, publicIPLabel)
			} else {
				node.Labels[publicIPLabel] = oldIP
			}
			_, updateErr := nodes.Update(ctx.Request.Context(), node, metav1.UpdateOptions{})
			return updateErr
		})
		if rollbackErr != nil {
			err = fmt.Errorf("tls-san 更新失败：%w；节点标签回滚失败：%v", err, rollbackErr)
		}
		self.JsonResponseWithServerError(ctx, err)
		return
	}
	self.JsonResponseWithoutError(ctx, gin.H{"ip": ip, "restartTriggered": configChanged})
}

func filterPublicSANs(value, oldIP string, public []string) string {
	addresses := []string{}
	for _, raw := range strings.Split(value, ",") {
		address := strings.TrimSpace(raw)
		if address == "" || address == oldIP {
			continue
		}
		builtIn := false
		for _, ip := range public {
			if address == ip {
				builtIn = true
				break
			}
		}
		if !builtIn {
			addresses = append(addresses, address)
		}
	}
	return strings.Join(addresses, ",")
}
