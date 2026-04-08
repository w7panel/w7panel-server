package buildimage

import (
	"errors"
	"os"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func panelRegistryServerHost() (string, error) {
	ip, err := panelRegistryServerIp()
	if err != nil {
		return "", err
	}
	return ip + ":8000", nil
}

func panelRegistryServerIp() (string, error) {
	if helper.IsK3kVirtual() {
		return os.Getenv("POD_IP"), nil
	}
	sdk := k8s.NewK8sClient()
	podlist, err := sdk.GetDaemonsetAgentPods("default")
	if err != nil {
		return "", err
	}
	for _, pod := range podlist.Items {
		if pod.Status.Phase == "Running" {
			panelDomain := pod.Status.PodIP
			return panelDomain, nil
		}

	}
	return "", errors.New("not found agent registry ip")
}
