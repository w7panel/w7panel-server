// nolint
package metrics

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

func TestGetResourceDiskUsage(t *testing.T) {

	// a1, b1, err := usge.GetResourceDiskUsage(types.NewK3kUser(sa))
}

func TestGetResourceUsage(t *testing.T) {

	usageApi := NewK3kUsage(k8s.NewK8sClient().Sdk)
	cvm, err := usageApi.getCvm("console-164315", "k3k-console-164315")
	if err != nil {
		t.Error(err)
		return
	}
	usage, total, err := usageApi.GetResourceCvmDiskUsage(cvm)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(total, usage)
	// a1, b1, c1, d1, err := usge.GetResourceUsage(types.NewK3kUser(sa))
}
