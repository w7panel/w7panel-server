package buildimage

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"go.yaml.in/yaml/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var defaultRegistryMap = map[string]Mirror{
	"index.docker.io": Mirror{
		Endpoints: []string{
			"registry.cn-hangzhou.aliyuncs.com",
			"mirror.ccs.tencentyun.com",
			"docker.1panel.live",
			"docker.m.daocloud.io",
		},
	},
}

func mirrorMapToStr(panelHost string) string {
	result, err := getRegistryMapArr()
	if err != nil {
		def := mirrorsToKvArr(defaultRegistryMap)
		def = append(def, "registry.local.w7.cc="+panelHost)
		return strings.Join(def, ";")
	}
	result = append(result, "registry.local.w7.cc="+panelHost)
	return strings.Join(result, ";")
}
func readRegistryBytes() ([]byte, error) {
	if helper.IsK3kVirtual() {
		return os.ReadFile("/proc/1/root/etc/rancher/k3s/registries.yaml")
	}
	rootSdk := k8s.NewK8sClient()
	cfg, err := rootSdk.ClientSet.CoreV1().ConfigMaps("default").Get(context.Background(), "registries", metav1.GetOptions{})
	if err != nil {
		return []byte{}, err
	}
	registries := cfg.Data["default.cnf"]
	return []byte(registries), nil
}
func getRegistryMapArr() ([]string, error) {
	registries, err := readRegistryBytes()
	if err != nil {
		slog.Error("getRegistryMapArr yaml unmarshal error", "err", err)
		return mirrorsToKvArr(defaultRegistryMap), nil
	}
	reg := &Registry{}
	err = yaml.Unmarshal([]byte(registries), reg)
	if err != nil {
		slog.Error("getRegistryMapArr yaml unmarshal error", "err", err)
		return mirrorsToKvArr(defaultRegistryMap), nil
	}
	return mirrorsToKvArr(reg.Mirrors), nil
}

func mirrorsToKvArr(mirrors map[string]Mirror) []string {
	kvstr := []string{}
	for k, v := range mirrors {
		nkv := mirrorToKvArr(k, v)
		kvstr = append(kvstr, nkv...)
	}
	// result := strings.Join(kvstr, ";")
	return kvstr
}
func mirrorToKvArr(k string, mirror Mirror) []string {
	kvstr := []string{}
	if len(mirror.Endpoints) > 0 {
		for _, endpoint := range mirror.Endpoints {
			ed := strings.ReplaceAll(endpoint, "https://", "")
			ed = strings.ReplaceAll(ed, "http://", "")
			if k == "docker.io" {
				k = "index.docker.io"
			}
			kvstr = append(kvstr, k+"="+ed)
		}
	}
	return kvstr
}
