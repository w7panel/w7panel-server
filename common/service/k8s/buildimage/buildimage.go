package buildimage

import (
	"errors"
	"os"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func getPanelRegistryIp() (string, error) {
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

func ToJob(spec BuildImageSpec, currentSdk *k8s.Sdk) error {
	// ToJob := ToBuildJob(spec)
	return nil
}

// func defaultRegistryMap() string {
// 	return `index.docker.io=mirror.ccs.tencentyun.com;index.docker.io=registry.cn-hangzhou.aliyuncs.com;
// 	index.docker.io=docker.m.daocloud.io;index.docker.io=docker.1panel.live`
// }

func getRegistryMapStr(rootSdk *k8s.Sdk, panelIp string) (string, error) {

	panelIp, err := getPanelRegistryIp()
	if err != nil {
		return "", err
	}
	regArr, err := getRegistryMapArr()
	if err != nil {
		return "", err
	}
	regArr = append(regArr, "registry.local.w7.cc="+panelIp+":8000")
	return strings.Join(regArr, ";"), nil
}
