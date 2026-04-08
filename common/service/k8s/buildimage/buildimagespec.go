package buildimage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/w7panel/w7panel/common/helper"
	buildimagev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/buildimage/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const (
	defaultDomain = "registry.local.w7.cc"
)

type BuildImageSpec struct {
	*buildimagev1alpha1.BuildImageSpec
}

func (b *BuildImageSpec) GetBuildJobName() string {
	buildJobName := "zpk-" + strings.ToLower(helper.RandomString(10))
	if b.TaskID != "" {
		buildJobName = b.TaskID
	}
	return buildJobName
}
func (b *BuildImageSpec) GetPushDomain() string {
	ref, err := referenceutil.Parse(b.TargetImage.Address)
	if err != nil {
		return ""
	}
	return ref.Domain

}

// 实际推送地址
func (b *BuildImageSpec) GetRealPushImage(panelDomain string) string {
	if b.IsPushToDefault() {
		return strings.ReplaceAll(b.TargetImage.Address, "registry.local.w7.cc", panelDomain)
	}
	return b.TargetImage.Address
}

func (b *BuildImageSpec) IsPushToDefault() bool {
	ref, err := referenceutil.Parse(b.TargetImage.Address)
	if err != nil {
		return false
	}
	if ref.Domain == "registry.local.w7.cc" {
		return true
	}
	return false
}

func (m *BuildImageSpec) GetBuildContext() string {
	return "/workspace/"
}
func (m *BuildImageSpec) GetInsecure() string {
	if m.IsPushToDefault() {
		return "--insecure --insecure-pull"
	}
	return ""
}
func (d *BuildImageSpec) ToMap() map[string]string {

	return map[string]string{
		"USER_AGENT":  "release",
		"DOCKER_AUTH": d.GetAuthJsonString(),
		// "PUSH_IMAGE":            d.GetRealPushImage(),
		"INSECURE":              d.GetInsecure(),
		"DOWNLOAD_URL":          d.Source.DownloadURL,
		"NOTIFY_COMPLETION_URL": d.NotifyURL,
		"NOTIFY_FAILED_URL":     d.NotifyURL,
		"CURL_CA_BUNDLE":        "/kaniko/ssl/certs/ca-certificates.crt",
		"CONTEXT":               d.GetBuildContext(),
		"DOCKER_FILE":           d.Source.DockerfilePath,
		// "KANIKO_REGISTRY_MAP":   d.registryMap(),
		"EMBED": "false",
	}
}

func (d BuildImageSpec) ToEnv(registryHost string) []corev1.EnvVar {
	var envs []corev1.EnvVar
	for k, v := range d.ToMap() {
		envVar := corev1.EnvVar{Name: k, Value: v}
		envs = append(envs, envVar)
	}
	realPushImage := d.GetRealPushImage(registryHost)
	envs = append(envs, corev1.EnvVar{Name: "PUSH_IMAGE", Value: realPushImage})
	envs = append(envs, corev1.EnvVar{Name: "KANIKO_REGISTRY_MAP", Value: mirrorMapToStr()})
	return envs
}

func (d BuildImageSpec) GetAuthJsonString() string {

	auth := fmt.Sprintf("%s:%s", d.TargetImage.Auth.Username, d.TargetImage.Auth.Password)
	base64_encode := func(s string) string {
		return base64.StdEncoding.EncodeToString([]byte(s))
	}
	host := d.GetPushDomain()
	jsonVal := map[string]interface{}{
		"auths": map[string]interface{}{
			host: map[string]interface{}{
				"auth": base64_encode(auth),
			},
		},
	}
	result, err := json.Marshal(jsonVal)
	if err != nil {
		return ""
	}
	return string(result)
}
