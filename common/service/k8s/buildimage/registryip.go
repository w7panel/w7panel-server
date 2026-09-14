package buildimage

import (
	"errors"

	"github.com/w7panel/w7panel/common/service/k8s"
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
			if hostIp != "" {
				if hostIp == pod.Status.HostIP {
					panelDomain := pod.Status.PodIP
					return panelDomain, nil
				}
			} else {
				panelDomain := pod.Status.PodIP
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
	sdk, err := k8s.NewK8sClient().Channel(token)
	result := &RegServerInfo{RequestUrl: "/"}
	if err != nil {
		return result, err
	}
	podIp, err := panelRegistryServerIpUseSdk(sdk, hostIp)
	if err != nil {
		return result, err
	}
	result.RequestUrl = "/panel-api/v1/" + podIp + ":8000/proxy"
	result.RequestHost = podIp + ":8000"
	return result, nil
}
