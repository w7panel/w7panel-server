package buildimage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/w7panel/w7panel/common/helper"
)

const (
	defaultDomain = "registry.local.w7.cc"
)

type BuildImageSpec struct {
	TaskID    string `json:"taskId"`
	Namespace string `json:"namespace"`
	Source    struct {
		DownloadURL    string `json:"downloadUrl"`
		DockerfilePath string `json:"dockerfilePath"`
	} `json:"source"`
	TargetImage struct {
		Address string `json:"address"`
		Auth    struct {
			Username string `json:"username"`
			Password string `json:"password"`
		} `json:"auth"`
	} `json:"targetImage"`
	NotifyURL string `json:"notifyUrl"`
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
	ref, err := referenceutil.Parse(b.TargetImage.Address)
	if err != nil {
		return b.TargetImage.Address
	}
	if ref.Domain == defaultDomain {
		ref.Domain = panelDomain
	}
	return ref.String()
}

func (b *BuildImageSpec) ToEnv(panelDomain string) string {
	ref, err := referenceutil.Parse(b.TargetImage.Address)
	if err != nil {
		return b.TargetImage.Address
	}
	if ref.Domain == defaultDomain {
		ref.Domain = panelDomain
	}
	return ref.String()
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
	return ""
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

// func (d BuildImageSpec) ToEnv() []corev1.EnvVar {
// 	var envs []corev1.EnvVar
// 	for k, v := range d.ToMap() {
// 		envVar := corev1.EnvVar{Name: k, Value: v}
// 		envs = append(envs, envVar)
// 	}
// 	return envs
// }

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
