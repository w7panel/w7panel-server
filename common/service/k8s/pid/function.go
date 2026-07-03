package pid

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/terminal"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	pidsAnnotation         = "w7.cc/pids"
	containerIDsAnnotation = "w7.cc/container-ids"
	legacyPidAnnotation    = "w7.cc/pid"
	legacyCIDAnnotation    = "w7.cc/container-id"
)

// webhook 入口获取pid
func LoadPid(pod *corev1.Pod) (int, error) {
	//如果是主集群 转发请求到agent节点获取pid
	sdk := k8s.NewK8sClient()
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return 0, err
	}

	status, err := resolveContainerStatus(pod, "", "")
	if err != nil {
		return 0, err
	}
	if status.Ready == false {
		return 0, fmt.Errorf("cluster pod container %s is not running", status.Name)
	}
	containerId := status.ContainerID
	if helper.IsChildAgent() {
		if helper.IsK3kVirtual() {
			//os 执行命令
			cmd := []string{"inspect", "--output", "go-template", fmt.Sprintf("--template='{{.info.pid}}'"), containerId}
			output, err := exec.Command("crictl", cmd...).Output()
			if err != nil {
				slog.Error("run cmd err", "err", err)
				return 0, err
			}
			pid, err := bytesToPid(output)
			if err != nil {
				slog.Error("bytesToPid", "err", err)
				return 0, err
			}

			controllerutil.CreateOrPatch(sdk.Ctx, sigClient, pod, func() error {
				if pod.Annotations == nil {
					pod.Annotations = make(map[string]string)
				}
				return setAnnotationContainerPid(pod, status.Name, containerId, pid)
			})
			return pid, nil
		}
		if helper.IsK3kShared() {
			// 使用的主集群pod 不需要处理
		}
	} else {
		daemonsetPod, err := sdk.GetDaemonsetAgentPod(sdk.GetNamespace(), pod.Status.HostIP)
		if err != nil {
			slog.Error("get  daemonsetPod err", "err", err)
			return 0, err
		}
		if (len(pod.Status.ContainerStatuses) > 0) && containerId != "" {
			pid, err := GetPid(daemonsetPod, containerId, true, sdk.Sdk)
			if err != nil {
				return 0, err
			}
			controllerutil.CreateOrPatch(sdk.Ctx, sigClient, pod, func() error {
				if pod.Annotations == nil {
					pod.Annotations = make(map[string]string)
				}
				return setAnnotationContainerPid(pod, status.Name, containerId, pid)
			})
			return pid, nil
		}
	}
	return 0, errors.New("not found pid")
	//如果是子集群 直接通过当前shell获取
}
func GetPid(findPod *corev1.Pod, containerId string, nscener bool, sdk *k8s.Sdk) (int, error) {
	session := terminal.NewTerminalSession(nil)
	defer session.Close()
	containerName := findPod.Spec.Containers[0].Name

	containerId = normalizeContainerID(containerId)
	cmd := []string{"nsenter", "-t", "1", "--mount", "--pid", "--", "crictl", "inspect", "--output", "go-template", fmt.Sprintf("--template='{{.info.pid}}'"), containerId}
	if !nscener {
		cmd = []string{"crictl", "inspect", "--output", "go-template", fmt.Sprintf("--template='{{.info.pid}}'"), containerId}
	}

	err := sdk.RunExec(session, findPod.Namespace, findPod.Name, containerName, cmd, false)
	if err != nil {
		return 0, err
	}
	pid := string(session.GetWriterBytes())
	pid = strings.Replace(pid, "\n", "", -1)
	pid = strings.Replace(pid, "'", "", -1)
	//pid string to int
	pidInt, err := strconv.Atoi(pid)
	if err != nil {
		return 0, err
	}
	return pidInt, nil
}

func bytesToPid(data []byte) (int, error) {
	pid := string(data)
	pid = strings.Replace(pid, "\n", "", -1)
	pid = strings.Replace(pid, "'", "", -1)
	//pid string to int
	pidInt, err := strconv.Atoi(pid)
	if err != nil {
		return 0, err
	}
	return pidInt, nil
}

func GetContainerPid(agentPod *corev1.Pod, pod *corev1.Pod, containerName, containerId string, nscener bool, sdk *k8s.Sdk) (int, string, error) {
	status, err := resolveContainerStatus(pod, containerName, containerId)
	if err != nil {
		return 0, "", err
	}

	pid, err := getAnnotationPodPid(pod, status.Name, status.ContainerID)
	if err != nil {
		slog.Error("getAnnotationPodPid", "err", err)
	}
	if err == nil && pid != 0 {
		return pid, status.Name, nil
	}
	pid, err = GetPid(agentPod, status.ContainerID, nscener, sdk)
	if err != nil {
		return 0, "", err
	}
	return pid, status.Name, nil
}

func getAnnotationPodPid(pod *corev1.Pod, containerName, containerId string) (int, error) {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	containerIDs, err := getAnnotationMap(pod, containerIDsAnnotation)
	if err != nil {
		return 0, err
	}
	annoContainerId := containerIDs[containerName]
	if annoContainerId == "" {
		return 0, fmt.Errorf("not found annotation containerId for container %s", containerName)
	}
	if !sameContainerID(containerId, annoContainerId) {
		return 0, errors.New("containerId is not equal")
	}

	pids, err := getAnnotationMap(pod, pidsAnnotation)
	if err != nil {
		return 0, err
	}
	pidValue, ok := pids[containerName]
	if !ok {
		return 0, fmt.Errorf("not found annotation pid for container %s", containerName)
	}
	pidInt, err := strconv.Atoi(pidValue)
	if err != nil {
		return 0, err
	}
	return pidInt, nil
}

func checkPodRunning(pod *corev1.Pod, containerName, containerId string) error {
	if containerName == "" && containerId == "" {
		if len(pod.Status.ContainerStatuses) == 0 {
			return errors.New("not found pod container")
		}
		for _, status := range pod.Status.ContainerStatuses {
			if status.State.Running != nil || status.State.Terminated == nil {
				return nil
			}
		}
		return errors.New("cluster pod is not running")
	}
	status, err := resolveContainerStatus(pod, containerName, containerId)
	if err != nil {
		return err
	}
	if status.State.Running == nil && status.State.Terminated != nil {
		return fmt.Errorf("cluster pod container %s is not running", status.Name)
	}
	return nil
}

func resolveContainerStatus(pod *corev1.Pod, containerName, containerId string) (*corev1.ContainerStatus, error) {
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil, errors.New("not found pod container")
	}
	if containerName != "" {
		for i := range pod.Status.ContainerStatuses {
			if pod.Status.ContainerStatuses[i].Name == containerName {
				return &pod.Status.ContainerStatuses[i], nil
			}
		}
		return nil, fmt.Errorf("container %s not found", containerName)
	}
	if containerId != "" {
		for i := range pod.Status.ContainerStatuses {
			if sameContainerID(pod.Status.ContainerStatuses[i].ContainerID, containerId) {
				return &pod.Status.ContainerStatuses[i], nil
			}
		}
		return nil, fmt.Errorf("containerId %s not found", containerId)
	}
	if len(pod.Status.ContainerStatuses) == 1 {
		return &pod.Status.ContainerStatuses[0], nil
	}
	return nil, errors.New("pod has multiple containers, specify containerName or containerId")
}

func setAnnotationContainerPid(pod *corev1.Pod, containerName, containerId string, pid int) error {
	if pod.Annotations == nil {
		pod.Annotations = map[string]string{}
	}
	pids, err := getAnnotationMap(pod, pidsAnnotation)
	if err != nil {
		return err
	}
	containerIDs, err := getAnnotationMap(pod, containerIDsAnnotation)
	if err != nil {
		return err
	}
	pids[containerName] = strconv.Itoa(pid)
	containerIDs[containerName] = containerId
	if err := setAnnotationMap(pod, pidsAnnotation, pids); err != nil {
		return err
	}
	if err := setAnnotationMap(pod, containerIDsAnnotation, containerIDs); err != nil {
		return err
	}
	delete(pod.Annotations, legacyPidAnnotation)
	delete(pod.Annotations, legacyCIDAnnotation)
	return nil
}

func getAnnotationMap(pod *corev1.Pod, key string) (map[string]string, error) {
	result := map[string]string{}
	if pod.Annotations == nil || pod.Annotations[key] == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(pod.Annotations[key]), &result); err != nil {
		return nil, fmt.Errorf("parse annotation %s: %w", key, err)
	}
	return result, nil
}

func setAnnotationMap(pod *corev1.Pod, key string, value map[string]string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	pod.Annotations[key] = string(data)
	return nil
}

func sameContainerID(a, b string) bool {
	return normalizeContainerID(a) == normalizeContainerID(b)
}

func normalizeContainerID(containerId string) string {
	containerId = strings.TrimSpace(containerId)
	if index := strings.Index(containerId, "://"); index >= 0 {
		return containerId[index+3:]
	}
	return containerId
}
