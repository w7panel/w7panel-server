package metrics

import (
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/longhorn"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalUsage measures the Kubernetes cluster served locally by this panel.
type LocalUsage struct {
	sdk *k8s.Sdk
}

func NewLocalUsage(sdk *k8s.Sdk) *LocalUsage {
	return &LocalUsage{sdk: sdk}
}

func (k *LocalUsage) GetResourceUsage() (cpuUsage, memoryUsage resource.Quantity, allocatedCPU, allocatedMemory resource.Quantity, err error) {
	cpuUsage, memoryUsage = k.nodeMetricsUsage()
	allocatedCPU, allocatedMemory, err = k.nodeAllocate(allocatedCPU, allocatedMemory)
	return
}

func (k *LocalUsage) nodeMetricsUsage() (cpuUsage, memoryUsage resource.Quantity) {
	for _, metric := range NodeMetrics.GetLatestMetrics() {
		cpuUsage.Add(*resource.NewMilliQuantity(metric.CPUUsage, resource.DecimalSI))
		memoryUsage.Add(*resource.NewQuantity(metric.MemoryUsage, resource.BinarySI))
	}
	return cpuUsage, memoryUsage
}

func (k *LocalUsage) nodeAllocate(allocatedCPU resource.Quantity, allocatedMemory resource.Quantity) (resource.Quantity, resource.Quantity, error) {
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

func (k *LocalUsage) GetResourceDiskUsage() (storageUsage int64, storageTotal int64, err error) {
	longhornClient, err := longhorn.NewLonghornClient(k.sdk)
	if err != nil {
		return 0, 0, err
	}
	nodes, err := longhornClient.GetNodeList()
	if err != nil {
		return 0, 0, err
	}
	for _, node := range nodes.Items {
		for _, disk := range node.Status.DiskStatus {
			storageTotal += disk.StorageMaximum
			storageUsage += disk.StorageMaximum - disk.StorageAvailable
		}
	}
	return storageUsage, storageTotal, nil
}
