package pid

import (
	"context"
	"fmt"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/terminal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type pid struct {
	rootSdk *k8s.Sdk
}

func NewPid(token string) (*pid, error) {
	// PID and file operations are local to the panel agent serving this request.
	// The caller token has already been authenticated by the HTTP middleware; it
	// must not select another cluster or alter the Kubernetes client here.
	return &pid{rootSdk: k8s.NewK8sClient().Sdk}, nil
}

func NewPidTest(saName string) (*pid, error) {
	return NewPid("")
}

func (p *pid) Handle(param PidParam) (*PidResult, error) {
	if param.FromPodName == "" {
		return nil, fmt.Errorf("podName is required")
	}
	pod, err := p.rootSdk.ClientSet.CoreV1().Pods(param.Namespace).Get(context.Background(), param.FromPodName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if param.HostIp != "" && pod.Status.HostIP != param.HostIp {
		return nil, fmt.Errorf("target node is not local: requested %s, pod runs on %s", param.HostIp, pod.Status.HostIP)
	}
	if err := checkPodRunning(pod, param.FromPodContainerName, param.ContainerId); err != nil {
		return nil, err
	}
	status, err := resolveContainerStatus(pod, param.FromPodContainerName, param.ContainerId)
	if err != nil {
		return nil, err
	}
	if param.ContainerId != "" && !sameContainerID(param.ContainerId, status.ContainerID) {
		return nil, fmt.Errorf("containerId is not equal")
	}
	// File access is served by the privileged daemonset pod on the Pod's node.
	// This is still local-cluster traffic; it is not the former panel-to-child
	// cluster proxy path.
	agentPod, err := p.rootSdk.GetDaemonsetAgentPod(p.rootSdk.GetNamespace(), pod.Status.HostIP)
	if err != nil {
		return nil, err
	}
	if err := checkPodRunning(agentPod, "", ""); err != nil {
		return nil, err
	}
	pidValue, containerName, err := GetContainerPid(agentPod, pod, status.Name, status.ContainerID, true, p.rootSdk)
	if err != nil {
		return nil, err
	}
	_ = p.patchContainerPid(p.rootSdk, pod, containerName, status.ContainerID, pidValue)
	param.FromPodContainerName = containerName
	pwd := "/"
	if param.FromPodName != "" && param.FromPodContainerName != "" {
		pwd1, err := p.GetPwd(param)
		if err != nil {
			pwd = "/"
		} else {
			pwd = pwd1
		}
	}
	return &PidResult{
		Pid:           pidValue,
		ProxyIp:       agentPod.Status.PodIP,
		AgentPod:      agentPod,
		ContainerName: param.FromPodContainerName,
		Pwd:           pwd,
	}, nil

}

func (self *pid) patchContainerPid(sdk *k8s.Sdk, pod *corev1.Pod, containerName, containerId string, pid int) error {
	if sdk == nil || pod == nil || containerName == "" || pid == 0 {
		return nil
	}
	status, err := resolveContainerStatus(pod, containerName, containerId)
	if err != nil {
		return err
	}
	latest, err := sdk.ClientSet.CoreV1().Pods(pod.Namespace).Get(context.Background(), pod.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if latest.Annotations == nil {
		latest.Annotations = map[string]string{}
	}
	if err := setAnnotationContainerPid(latest, status.Name, status.ContainerID, pid); err != nil {
		return err
	}
	_, err = sdk.ClientSet.CoreV1().Pods(latest.Namespace).Update(context.Background(), latest, metav1.UpdateOptions{})
	return err
}

func (self *pid) GetPwd(params PidParam) (string, error) {
	session := terminal.NewTerminalSession(nil)
	defer session.Close()
	err := self.rootSdk.RunExec(session, params.Namespace, params.FromPodName, params.FromPodContainerName, []string{"pwd"}, false)
	if err != nil {
		return "", err
	}
	return string(session.GetWriterBytes()), nil
}
