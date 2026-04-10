package buildimage

import (
	"context"
	"errors"
	"os"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func panelRegistryServerHost() (string, error) {
	ip, err := panelRegistryServerIp()
	if err != nil {
		return "", err
	}
	return ip + ":8000", nil
}

// operator 运行环境 判断
func panelRegistryServerIp() (string, error) {
	if helper.IsK3kVirtual() {
		return os.Getenv("POD_IP"), nil
	}
	sdk := k8s.NewK8sClient()
	return panelRegistryServerIpUseSdk(sdk.Sdk, "")
}

// controller zpk.go 中直接使用sdk获取
func panelRegistryServerIpUseSdk(sdk *k8s.Sdk, hostIp string) (string, error) {
	podlist, err := sdk.GetDaemonsetAgentPods("default")
	if err != nil {
		return "", err
	}
	for _, pod := range podlist.Items {
		if pod.Status.Phase == "Running" {
			if hostIp != "" && hostIp == pod.Status.HostIP {
				panelDomain := pod.Status.PodIP
				return panelDomain, nil
			} else {
				panelDomain := pod.Status.PodIP
				return panelDomain, nil
			}

		}
	}
	// 子集群pod
	pods2List, err := sdk.ClientSet.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: "k3k-agent-pod=true"})
	for _, pod2 := range pods2List.Items {
		if pod2.Status.Phase == "Running" {
			if hostIp != "" && hostIp == pod2.Status.HostIP {
				panelDomain := pod2.Status.PodIP
				return panelDomain, nil
			} else {
				panelDomain := pod2.Status.PodIP
				return panelDomain, nil
			}
		}
	}
	return "", errors.New("not found agent registry ip")
}

func PanelRegistryServerHostUseSdk(sdk *k8s.Sdk) (string, error) {
	ip, err := panelRegistryServerIpUseSdk(sdk, "")
	if err != nil {
		return "", err
	}
	return ip + ":8000", nil
}

func PanelRegistryServerInfo(token string, hostIp string) (*RegServerInfo, error) {
	tokenObj := k8s.NewK8sToken(token)
	sdk, err := k8s.NewK8sClient().Channel(token)
	result := &RegServerInfo{RequestUrl: "/"}
	if err != nil {
		return result, err
	}
	podIp, err := panelRegistryServerIpUseSdk(sdk, hostIp)
	if err != nil {
		return result, err
	}
	if !tokenObj.IsK3kCluster() {
		result.RequestUrl = "/panel-api/v1/" + podIp + ":8000/proxy"
	}
	result.RequestHost = podIp + ":8000"
	return result, nil
}
