package webhook

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/w7panel/w7panel/common/helper"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/w7panel/w7panel/common/service/k8s/pid"
	corev1 "k8s.io/api/core/v1"
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
	if pod.Namespace == "default" {
		pid.WebHookPid(pod.DeepCopy()) //default 命名空间下，才执行 pid 注入
	}

	modified := false

	if helper.IsLxcfsEnabled() {
		modified = injectLxcfs(pod)
	}
	if isLxcfsAnnotationEnabled(pod) {
		modified = injectLxcfs(pod) || modified
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

func isLxcfsAnnotationEnabled(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations["w7.cc/lxcfs"] == "true"
}

func injectLxcfs(pod *corev1.Pod) bool {
	modified := false
	// https://github.com/ymping/lxcfs-admission-webhook/blob/main/cmd/volume.go
	for _, volume := range volumesTemplate {
		if !hasVolume(pod.Spec.Volumes, volume.Name) {
			pod.Spec.Volumes = append(pod.Spec.Volumes, volume)
			modified = true
		}
	}
	for i := range pod.Spec.Containers {
		for _, volumeMount := range volumeMountsTemplate {
			if !hasVolumeMount(pod.Spec.Containers[i].VolumeMounts, volumeMount.Name, volumeMount.MountPath) {
				pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, volumeMount)
				modified = true
			}
		}
	}
	return modified
}

func hasVolume(volumes []corev1.Volume, name string) bool {
	for _, volume := range volumes {
		if volume.Name == name {
			return true
		}
	}
	return false
}

func hasVolumeMount(volumeMounts []corev1.VolumeMount, name string, mountPath string) bool {
	for _, volumeMount := range volumeMounts {
		if volumeMount.Name == name || volumeMount.MountPath == mountPath {
			return true
		}
	}
	return false
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
