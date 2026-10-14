package pid

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

type PidParam struct {
	Namespace            string `form:"namespace" binding:"required"`
	HostIp               string `form:"HostIp" binding:"required"`
	ContainerId          string `form:"containerId"`
	FromPodName          string `form:"podName"`       //原始pod名
	FromPodContainerName string `form:"containerName"` //原始pod container名
}

type PidResult struct {
	Pid           int    `json:"pid"`
	SubPid        int    `json:"subPid"`
	ProxyIp       string `json:"proxyIp"`
	ProxyPort     int    `json:"proxyPort"`
	AgentPod      *corev1.Pod
	ContainerName string `json:"containerName"`
	Pwd           string `json:"pwd"`
}

type PidCacheItem struct {
	podName     string
	namespace   string
	containerId string
	pid         int
}

func (p *PidResult) ToArray() map[string]string {
	podIp := p.ProxyIp
	pidstr := strconv.Itoa(p.Pid)
	subpidstr := strconv.Itoa(p.SubPid)

	proxyPortStr := "8000"
	if p.ProxyPort > 0 {
		proxyPortStr = strconv.Itoa(p.ProxyPort)
	}
	// Requests without an agent Pod IP are handled by the local agent endpoint.
	webdavUrl := "/panel-api/v1/files/webdav-agent/" + pidstr + "/agent"
	webdavBasePath := "panel-api/v1/files/webdav-agent/" + pidstr + "/agent" //前端根据这个过滤掉 当前目录?
	compressUrl := "/panel-api/v1/files/compress-agent/" + pidstr
	permissionUrl := "/panel-api/v1/files/permission-agent/" + pidstr

	// An agent Pod IP allows direct forwarding to that node's daemonset agent.
	if podIp != "" {
		webdavUrl = "/panel-api/v1/" + podIp + ":" + proxyPortStr + "/proxy/panel-api/v1/files/webdav-agent/" + pidstr + "/agent"
		webdavBasePath = "panel-api/v1/files/webdav-agent/" + pidstr + "/agent" //前端根据这个过滤掉 当前目录?
		compressUrl = "/panel-api/v1/" + podIp + ":" + proxyPortStr + "/proxy/panel-api/v1/files/compress-agent/" + pidstr
		permissionUrl = "/panel-api/v1/" + podIp + ":" + proxyPortStr + "/proxy/panel-api/v1/files/permission-agent/" + pidstr
	}
	pod := p.AgentPod //节点管理 不再返回agent pod 直接节点ip:9090 访问
	if subpidstr == "0" {
		subpidstr = ""
	}
	containerName := p.ContainerName
	if pod != nil && containerName == "" && len(pod.Spec.Containers) == 1 {
		containerName = pod.Spec.Containers[0].Name
	}

	ns := "default"
	podName := "default"
	if pod != nil {
		ns = pod.Namespace
		podName = pod.Name
	}
	return map[string]string{
		"podName":       podName,
		"pid":           pidstr,
		"subPid":        subpidstr,
		"namespace":     ns,
		"containerName": containerName,
		"podIp":         podIp,
		// "pwd":            pwd,
		"webdavUrl": webdavUrl,
		// "webdavToken":    token,
		"webdavBasePath": webdavBasePath,
		"compressUrl":    compressUrl,
		"permissionUrl":  permissionUrl,
		"pwd":            p.Pwd,
		"agentUrl":       "/panel-api/v1/" + podIp + ":" + proxyPortStr + "/proxy",
		// "users":          users,
	}

}
