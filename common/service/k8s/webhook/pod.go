package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/w7panel/w7panel/common/service/k8s/pid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (m *ResourceMutator) handlePod(ctx context.Context, req admission.Request) admission.Response {
	slog.Info("处理 Pod admission 请求")

	// 解码请求中的 Pod 资源
	pod := &corev1.Pod{}
	if err := (m.decoder).Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	if pod.Annotations != nil {
		val, ok := pod.Annotations[k3ktypes.W7_CREATE_POD]
		if ok && val == "false" {
			return admission.Denied("不允许创建pod")
		}
	}
	pid.WebHookPid(pod.DeepCopy())
	// // 检查 Pod 是否有 ownerReferences.kind=Cluster
	// _, isClusterNormalPod := pod.Labels["k3k.io/clusterName"]
	// if isClusterNormalPod {
	// 	return m.handleNormalPod(ctx, pod, req)
	// }
	// 纯普通pod
	modified := false
	// 新版cluster spec 直接指定limit
	// namespace := pod.Namespace
	// if strings.HasPrefix(namespace, "k3k-") && !helper.IsChildAgent() {
	// 	err := handlePodLabel(m.client, m.sdk, pod, namespace)
	// 	if err == nil {
	// 		modified = true
	// 	}
	// 	err = handlePodLimit(pod)//
	// 	if err == nil {
	// 		modified = true
	// 	}
	// }
	if helper.IsLxcfsEnabled() {
		//https://github.com/ymping/lxcfs-admission-webhook/blob/main/cmd/volume.go
		pod.Spec.Volumes = append(pod.Spec.Volumes, volumesTemplate...)
		for i := range pod.Spec.Containers {
			pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, volumeMountsTemplate...)
		}
		modified = true
	}
	if !modified {
		return admission.Allowed("Pod cpu memory 无需配置")
	}
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	// 返回修改后的资源
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// 添加维护标签
func handlePodLabel(client sigclient.Client, sdk *k8s.Sdk, pod *corev1.Pod, namespace string) error {
	cvm, err := k3k.GetCvm(context.TODO(), sdk, namespace, strings.ReplaceAll(namespace, "k3k-", ""))
	if err != nil {
		slog.Error("webhook pod 获取cvm失败", "err", err)
		return err
	}
	if cvm.Spec.Rescue {
		pod.Labels["w7.cc/weihu"] = "true"
	}
	return nil
}

func handlePodLimit(pod *corev1.Pod) error {
	for i := range pod.Spec.Containers {
		rs := pod.Spec.Containers[i].Resources
		if rs.Requests == nil {
			rs.Requests = make(corev1.ResourceList)
		}
		rs.Requests["cpu"] = resource.MustParse("0")
		rs.Requests["memory"] = resource.MustParse("0")
	}
	return nil
}

var volumeMountsTemplate = []corev1.VolumeMount{

	{
		Name:      "lxcfs-proc-cpuinfo",
		MountPath: "/proc/cpuinfo",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-meminfo",
		MountPath: "/proc/meminfo",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-diskstats",
		MountPath: "/proc/diskstats",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-stat",
		MountPath: "/proc/stat",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-swaps",
		MountPath: "/proc/swaps",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-uptime",
		MountPath: "/proc/uptime",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-proc-loadavg",
		MountPath: "/proc/loadavg",
		ReadOnly:  true,
	},
	{
		Name:      "lxcfs-sys-devices-system-cpu-online",
		MountPath: "/sys/devices/system/cpu/online",
		ReadOnly:  true,
	},
}
var hostfiletype corev1.HostPathType

func init() {
	hostfiletype = corev1.HostPathFile
}

var volumesTemplate = []corev1.Volume{
	{
		Name: "lxcfs-proc-cpuinfo",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/cpuinfo",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-diskstats",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/diskstats",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-meminfo",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/meminfo",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-stat",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/stat",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-swaps",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/swaps",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-uptime",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/uptime",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-proc-loadavg",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/proc/loadavg",
				Type: &hostfiletype,
			},
		},
	},
	{
		Name: "lxcfs-sys-devices-system-cpu-online",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/lib/lxcfs/sys/devices/system/cpu/online",
				Type: &hostfiletype,
			},
		},
	},
}
