package pid

import (
	"context"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/terminal"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type pid struct {
	rootSdk          *k8s.Sdk
	clientSdk        *k8s.Sdk
	isVirtual        bool
	virtualNamespace string
}

func NewPid(token string) (*pid, error) {
	tokenObj := k8s.NewK8sToken(token)
	var client *k8s.Sdk
	ns := "default"
	if tokenObj.IsK3kCluster() {
		clientSdk, err := k8s.NewK8sClient().Channel(token)
		if err != nil {
			return nil, err
		}
		client = clientSdk
		ns = tokenObj.GetNamespace()
	}
	root := k8s.NewK8sClient()

	return &pid{rootSdk: root.Sdk, clientSdk: client, isVirtual: tokenObj.IsK3kCluster(), virtualNamespace: ns}, nil
}

func NewPidTest(saName string) (*pid, error) {
	root := k8s.NewK8sClient()
	var client *k8s.Sdk
	isVirtual := false
	if saName != "" {
		k3kConfig := k8s.NewK3kConfig(saName, "k3k-"+saName, "http://test.cc", "")
		clientSdk, err := root.GetK3kClusterSdkByConfig(k3kConfig)
		if err != nil {
			return nil, err
		}
		client = clientSdk
		isVirtual = true
	}
	return &pid{rootSdk: root.Sdk, clientSdk: client, isVirtual: isVirtual, virtualNamespace: "k3k-" + saName}, nil
}

// 不能在子集群执行
func (p *pid) Handle(param PidParam) (*PidResult, error) {
	pid := 1 //节点文件管理默认1
	subPid := 0
	proxyIp := ""
	var agentPod *corev1.Pod
	if p.isVirtual {

		podfindApi := newPodFind(p.rootSdk.ClientSet, p.clientSdk.ClientSet)
		clusterPod, err := podfindApi.GetVirtualClusterNodePod(p.virtualNamespace, param.HostIp)
		if err != nil {
			return nil, err
		}
		err = checkPodRunning(clusterPod, "", "")
		if err != nil {
			return nil, err
		}
		// clusterPod.Status.HostIP 就是主集群pod ip
		daemonsetPod, err := p.rootSdk.GetDaemonsetAgentPod(p.rootSdk.GetNamespace(), clusterPod.Status.HostIP)
		if err != nil {
			slog.Error("get  daemonsetPod err", "err", err)
			return nil, err
		}
		err = checkPodRunning(daemonsetPod, "", "")
		if err != nil {
			return nil, err
		}

		// clusterPodContainerId := clusterPod.Status.ContainerStatuses[0].ContainerID
		// clusterPodPid, err := GetContainerPid(daemonsetPod, clusterPod, clusterPodContainerId, true, p.rootSdk) //20260616 新修改
		// // clusterPodPid, err := GetContainerPid(daemonsetPod, clusterPod, param.ContainerId, false, p.rootSdk) 之前为啥对 //TODO ???
		// if err != nil {
		// 	return nil, err
		// }

		// pid = clusterPodPid
		if param.FromPodName != "" {
			//为啥前端传containerId 为了获取pid, 后期因要从annnatation获取pid缓存, 所以需要查询k3kInnerPod
			k3kInnerPod, err := podfindApi.GetFromPod(param.FromPodName, param.Namespace, true)
			if err != nil {
				return nil, err
			}
			k3kInnerPodPid, containerName, err := GetContainerPid(clusterPod, k3kInnerPod, param.FromPodContainerName, param.ContainerId, false, p.rootSdk) //必须rootsdk
			if err != nil {
				return nil, err
			}
			_ = p.patchContainerPid(p.clientSdk, k3kInnerPod, containerName, param.ContainerId, k3kInnerPodPid)
			pid = k3kInnerPodPid
			param.FromPodContainerName = containerName
		}
		agentPod = clusterPod
		// proxyIp = daemonsetPod.Status.PodIP //20260624 子集群直接使用middleware proxy.go来转发请求
	} else {
		podfindApi := newPodFind(p.rootSdk.ClientSet, p.rootSdk.ClientSet)
		daemonsetPod, err := p.rootSdk.GetDaemonsetAgentPod(p.rootSdk.GetNamespace(), param.HostIp)
		if err != nil {
			slog.Error("get  daemonsetPod err", "err", err)
			return nil, err
		}
		if param.FromPodName != "" {

			//为啥前端传containerId 为了获取pid, 后期因要从annnatation获取pid缓存, 所以需要查询k3kInnerPod
			rootPod, err := podfindApi.GetFromPod(param.FromPodName, param.Namespace, true)
			if err != nil {
				return nil, err
			}
			rootPid, containerName, err := GetContainerPid(daemonsetPod, rootPod, param.FromPodContainerName, param.ContainerId, true, p.rootSdk)
			if err != nil {
				return nil, err
			}
			_ = p.patchContainerPid(p.rootSdk, rootPod, containerName, param.ContainerId, rootPid)
			pid = rootPid
			param.FromPodContainerName = containerName

		}
		agentPod = daemonsetPod
		proxyIp = daemonsetPod.Status.PodIP

	}
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
		Pid:           pid,
		SubPid:        subPid,
		ProxyIp:       proxyIp,
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
	sdk := self.rootSdk
	if self.isVirtual {
		sdk = self.clientSdk
	}
	err := sdk.RunExec(session, params.Namespace, params.FromPodName, params.FromPodContainerName, []string{"pwd"}, false)
	if err != nil {
		return "", err
	}
	return string(session.GetWriterBytes()), nil
}
