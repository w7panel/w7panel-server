package pid

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

func Test_MountFiles(t *testing.T) {
	sdk := k8s.NewK8sClient().Sdk

	fobj := NewMountFiles(sdk)
	result, err := fobj.Handle(MountFilesParam{
		Kind:           "Deployment",
		APIVersion:     "apps/v1",
		Name:           "w7-zpkv2-registry",
		Namespace:      "default",
		IncludeContent: false,
	})
	if err != nil {
		t.Error(err)
	}
	t.Log(result)
}
