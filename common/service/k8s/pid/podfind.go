package pid

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type podFind struct {
	root   *kubernetes.Clientset
	client *kubernetes.Clientset
}

func newPodFind(root, client *kubernetes.Clientset) *podFind {
	return &podFind{root: root, client: client}
}

func (f *podFind) GetVirtualClusterNodePod(namespace, hostIp string) (*corev1.Pod, error) {

	// ckmName := k3kConfig.CvmName
	// podName := k3kConfig.GetK3kServer0Name()
	pods, err := f.root.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		slog.Error("tty k3k list pods error", "err", err)
		return nil, err
	} else {

	}
	for _, pod := range pods.Items {
		if pod.Status.PodIP == hostIp { //PodIP 就是虚拟集群的节点IP
			return &pod, nil
		}
	}
	return nil, fmt.Errorf("not found virtual cluster pod")
}

func (f *podFind) GetPanelAgentPod(hostIp string) (*corev1.Pod, error) {
	dsPods, err := f.getRootClusterDaemonPodList()
	if err != nil {
		slog.Error("not find dspods", "err", err)
		return nil, err
	}
	for _, pod := range dsPods.Items {
		if pod.Status.HostIP == hostIp {
			return &pod, nil
		}
	}
	return nil, fmt.Errorf("not found pod")
}

// huoq
func (f *podFind) getRootClusterDaemonPodList() (*corev1.PodList, error) {
	daemonsetPods, err := f.root.CoreV1().Pods("default").List(context.TODO(), metav1.ListOptions{LabelSelector: "w7.cc/daemonset=w7"})
	if err != nil {
		slog.Warn("get daemonset pods error", "err", err)
		return nil, err
	}
	return daemonsetPods, nil
}

func (f *podFind) GetFromPod(fromPodName, namespace string, isVirtual bool) (*corev1.Pod, error) {
	if fromPodName == "" {
		return nil, fmt.Errorf("fromPodName is empty")
	}
	if namespace == "" {
		namespace = "default"
	}
	if isVirtual {
		return f.client.CoreV1().Pods(namespace).Get(context.Background(), fromPodName, metav1.GetOptions{})
	}
	return f.root.CoreV1().Pods(namespace).Get(context.Background(), fromPodName, metav1.GetOptions{})
}
