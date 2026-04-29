package metrics

import (
	"context"
	"fmt"
	"strconv"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type K3kUsage struct {
	sdk *k8s.Sdk
}

func NewK3kUsage(sdk *k8s.Sdk) *K3kUsage {
	return &K3kUsage{
		sdk: sdk,
	}
}

// GetResourceUsage returns the CPU and memory usage for a user, along with the total percentage of allocated resources.
func (k *K3kUsage) GetResourceUsage(k8stoken *k8s.K8sToken) (cpuUsage, memoryUsage resource.Quantity, allocatedCPU, allocatedMemory resource.Quantity, err error) {
	if k8stoken.IsK3kCluster() {
		cfg, err := k8stoken.GetK3kConfig()
		if err != nil {
			return cpuUsage, memoryUsage, allocatedCPU, allocatedMemory, err
		}
		client, err := k8s.NewK8sClient().GetK3kClusterSdkByConfig(cfg)
		if err != nil {
			return resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, nil
		}
		configmap, err := client.ClientSet.CoreV1().ConfigMaps("default").Get(client.Ctx, "metrics", metav1.GetOptions{})
		if err != nil {
			return resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, nil
		}
		cpuValue, _ := strconv.ParseInt(configmap.Data["cpu"], 10, 64)
		memoryValue, _ := strconv.ParseInt(configmap.Data["memory"], 10, 64)
		cpuUsage = *resource.NewMilliQuantity(cpuValue, resource.DecimalSI)
		memoryUsage = *resource.NewQuantity(memoryValue, resource.BinarySI)

	} else {
		// Get node metrics
		nodeMetrics := NodeMetrics.GetLatestMetrics()
		for _, metric := range nodeMetrics {
			cpuUsage.Add(*resource.NewMilliQuantity(metric.CPUUsage, resource.DecimalSI))
			memoryUsage.Add(*resource.NewQuantity(metric.MemoryUsage, resource.BinarySI))
		}
	}

	// Get allocated resources
	// var allocatedCPU, allocatedMemory resource.Quantity
	if k8stoken.IsK3kCluster() {
		cvm, err := k.getCvm(k8stoken.GetCvmName(), k8stoken.GetNamespace())
		if err != nil {
			return cpuUsage, memoryUsage, allocatedCPU, allocatedMemory, nil
		}
		allocatedCPU = resource.MustParse(fmt.Sprintf("%d", cvm.Status.EffectiveResource.CPU))
		allocatedMemory = resource.MustParse(fmt.Sprintf("%dGi", cvm.Status.EffectiveResource.Memory))
		if allocatedCPU.IsZero() || allocatedMemory.IsZero() {
			allocatedCPU, allocatedMemory, _ = k.nodeAllocate(allocatedCPU, allocatedMemory)
		}
	} else {
		allocatedCPU, allocatedMemory, _ = k.nodeAllocate(allocatedCPU, allocatedMemory)
	}

	return cpuUsage, memoryUsage, allocatedCPU, allocatedMemory, nil
}

// 救援模式token 不是cvm token 是用户token 需要单独查cvm
func (k *K3kUsage) GetResourceCvmUsage(cvm *cvmv1alpha1.Cvm) (cpuUsage, memoryUsage resource.Quantity, allocatedCPU, allocatedMemory resource.Quantity, err error) {

	cfg := &k8s.K3kConfig{
		Name:      cvm.GetK3kName(),
		Namespace: cvm.GetNamespace(),
		ApiServer: "",
		CvmName:   cvm.Name,
	}
	client, err := k8s.NewK8sClient().GetK3kClusterSdkByConfig(cfg)
	if err != nil {
		return resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, nil
	}
	configmap, err := client.ClientSet.CoreV1().ConfigMaps("default").Get(client.Ctx, "metrics", metav1.GetOptions{})
	if err != nil {
		return resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, resource.Quantity{}, nil
	}
	cpuValue, _ := strconv.ParseInt(configmap.Data["cpu"], 10, 64)
	memoryValue, _ := strconv.ParseInt(configmap.Data["memory"], 10, 64)
	cpuUsage = *resource.NewMilliQuantity(cpuValue, resource.DecimalSI)
	memoryUsage = *resource.NewQuantity(memoryValue, resource.BinarySI)

	allocatedCPU = resource.MustParse(fmt.Sprintf("%d", cvm.Status.EffectiveResource.CPU))
	allocatedMemory = resource.MustParse(fmt.Sprintf("%dGi", cvm.Status.EffectiveResource.Memory))
	if allocatedCPU.IsZero() || allocatedMemory.IsZero() {
		allocatedCPU, allocatedMemory, _ = k.nodeAllocate(allocatedCPU, allocatedMemory)
	}

	return cpuUsage, memoryUsage, allocatedCPU, allocatedMemory, nil
}
func (k *K3kUsage) getCvm(cvmName, ns string) (*cvmv1alpha1.Cvm, error) {
	cvm := &cvmv1alpha1.Cvm{}
	sigClient, err := k.sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	err = sigClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: cvmName}, cvm)
	if err != nil {
		return nil, err
	}
	return cvm, nil
}
func (k *K3kUsage) nodeAllocate(allocatedCPU resource.Quantity, allocatedMemory resource.Quantity) (resource.Quantity, resource.Quantity, error) {
	nodes, err := k.sdk.ClientSet.CoreV1().Nodes().List(k.sdk.Ctx, metav1.ListOptions{})
	if err != nil {
		return resource.Quantity{}, resource.Quantity{}, err
	}

	for _, node := range nodes.Items {
		allocatedCPU.Add(*node.Status.Allocatable.Cpu())
		allocatedMemory.Add(*node.Status.Allocatable.Memory())
	}
	return allocatedCPU, allocatedMemory, nil
}

func (k *K3kUsage) GetResourceDiskUsage(k8stoken *k8s.K8sToken) (storageUsage int64, storageTotal int64, err error) {
	// scName := k3kuser.GetStorageClass()

	longhornClient, err := longhorn.NewLonghornClient(k.sdk)
	if err != nil {
		return 0, 0, err
	}
	volumes, err := longhornClient.GetVolumeList()
	if err != nil {
		return 0, 0, err
	}
	pvcsizeMap := make(map[string]int64)
	for _, volume := range volumes.Items {
		if volume.Status.KubernetesStatus.PVCName == "" {
			continue
		}
		pvcsizeMap[volume.Status.KubernetesStatus.PVCName+":"+volume.Status.KubernetesStatus.Namespace] = volume.Status.ActualSize
	}

	if k8stoken.IsK3kCluster() {
		cvm, err := k.getCvm(k8stoken.GetCvmName(), k8stoken.GetNamespace())
		if err != nil {
			return 0, 0, err
		}
		return k.GetResourceCvmDiskUsage(cvm)
	} else {
		nodes, err := longhornClient.GetNodeList()
		if err != nil {
			return 0, 0, err
		}
		usage := int64(0)
		total := int64(0)
		for _, node := range nodes.Items {
			for _, storageNode := range node.Status.DiskStatus {
				total += storageNode.StorageMaximum
				usage += storageNode.StorageMaximum - storageNode.StorageAvailable
			}
		}
		return usage, total, nil
	}
}

// cvm 获取cvm的磁盘使用情况
func (k *K3kUsage) GetResourceCvmDiskUsage(cvm *cvmv1alpha1.Cvm) (storageUsage int64, storageTotal int64, err error) {
	// scName := k3kuser.GetStorageClass()

	longhornClient, err := longhorn.NewLonghornClient(k.sdk)
	if err != nil {
		return 0, 0, err
	}
	volumes, err := longhornClient.GetVolumeList()
	if err != nil {
		return 0, 0, err
	}
	pvcsizeMap := make(map[string]int64)
	for _, volume := range volumes.Items {
		if volume.Status.KubernetesStatus.PVCName == "" {
			continue
		}
		pvcsizeMap[volume.Status.KubernetesStatus.PVCName+":"+volume.Status.KubernetesStatus.Namespace] = volume.Status.ActualSize
	}

	total := resource.MustParse(fmt.Sprintf("%dGi", cvm.Status.EffectiveResource.Storage))
	pvcs, err := k.sdk.ClientSet.CoreV1().PersistentVolumeClaims(cvm.Namespace).List(k.sdk.Ctx, metav1.ListOptions{LabelSelector: "cluster=" + cvm.Name})
	if err != nil {
		return 0, 0, err
	}
	usage := int64(0)
	for _, pvc := range pvcs.Items {
		size, ok := pvcsizeMap[pvc.GetName()+":"+pvc.GetNamespace()]
		if ok {
			usage += size
		}
	}
	return usage, total.Value(), nil
}
