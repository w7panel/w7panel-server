package pid

import (
	"log/slog"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func WebHookPid(pod *corev1.Pod) {
	time.AfterFunc(time.Second*5, func() {
		if len(pod.Status.ContainerStatuses) != 1 {
			return
		}
		if pod.Annotations == nil {
			pod.Annotations = map[string]string{}
		}
		//如果已经设置 直接返回
		status := pod.Status.ContainerStatuses[0]
		containerIDs, err := getAnnotationMap(pod, containerIDsAnnotation)
		if err == nil && sameContainerID(containerIDs[status.Name], status.ContainerID) {
			return
		}
		_, err = LoadPid(pod)
		if err != nil {
			slog.Error("load pid error", "error", err)
		}
	})

}
