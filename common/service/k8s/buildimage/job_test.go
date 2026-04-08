package buildimage

import (
	"context"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	buildimagev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/buildimage/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToJbo(t *testing.T) {

	spec := &BuildImageSpec{
		BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
			TaskID:    "test-2",
			Namespace: "default",
			Source: buildimagev1alpha1.Source{
				DockerfilePath: "Dockerfile",
				DownloadURL:    "http://172.16.1.162:9090/ui/microapp/ddd2.zip",
			},
			TargetImage: buildimagev1alpha1.TargetImage{
				Address: "registry.local.w7.cc/w7panel/test-1:latest",
				Auth: buildimagev1alpha1.Auth{
					Username: "w7panel",
					Password: "w7panel",
				},
			},
		},
	}
	job, err := toBuildJob(spec)
	if err != nil {
		t.Errorf("toBuildJob() error = %v", err)
		return
	}
	sdk := k8s.NewK8sClient()
	_, err = sdk.ClientSet.BatchV1().Jobs("default").Create(context.Background(), job, v1.CreateOptions{})
	if err != nil {
		t.Errorf("create() error = %v", err)
		return
	}
}
