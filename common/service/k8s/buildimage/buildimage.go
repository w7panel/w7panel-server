package buildimage

import (
	"context"
	"errors"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	"go.yaml.in/yaml/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getPanelRegistryIp(sdk *k8s.Sdk) (string, error) {
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

func defaultRegistryMap() string {
	return `index.docker.io=mirror.ccs.tencentyun.com;index.docker.io=registry.cn-hangzhou.aliyuncs.com;
	index.docker.io=docker.m.daocloud.io;index.docker.io=docker.1panel.live`
}

func getRegistryMapStr(rootSdk *k8s.Sdk, panelIp string) string {
	result, err := getRegistryMapArr(rootSdk)
	if err != nil {
		return defaultRegistryMap() + ";registry.local.w7.cc=http://`+panelIp+`:8000"
	}
	return strings.Join(result, ";") + ";registry.local.w7.cc=http://`+panelIp+`:8000"
}
func getRegistryMapArr(rootSdk *k8s.Sdk) ([]string, error) {

	cfg, err := rootSdk.ClientSet.CoreV1().ConfigMaps("default").Get(context.Background(), "registries", metav1.GetOptions{})
	if err != nil {
		return []string{}, err
	}
	registries := cfg.Data["default.cnf"]
	reg := &Registry{}
	err = yaml.Unmarshal([]byte(registries), reg)
	if err != nil {
		return []string{}, err
	}
	kvstr := []string{}
	mirrors := reg.Mirrors
	for k, v := range mirrors {
		if len(v.Endpoints) > 0 {
			for _, endpoint := range v.Endpoints {
				kvstr = append(kvstr, k+"="+endpoint)
			}
		}
	}
	// result := strings.Join(kvstr, ";")
	return kvstr, nil
}
