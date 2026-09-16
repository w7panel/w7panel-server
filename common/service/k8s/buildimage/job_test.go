package buildimage

import (
	"context"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	buildimagev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/buildimage/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLocalRegistryJobUsesServiceAccountToken(t *testing.T) {
	spec := &BuildImageSpec{BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
		TaskID:             "local-registry",
		Namespace:          "default",
		ServiceAccountName: "founder",
		TargetImage: buildimagev1alpha1.TargetImage{
			Address: "registry.local.w7.cc/demo/app:latest",
		},
	}}
	job, err := toBuildJob(context.WithValue(context.Background(), PanelRegistryServerHostKey, "127.0.0.1:8000"), spec)
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Template.Spec.ServiceAccountName != "founder" {
		t.Fatalf("serviceAccountName = %q", job.Spec.Template.Spec.ServiceAccountName)
	}
	if job.Spec.Template.Spec.AutomountServiceAccountToken == nil || !*job.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Fatal("local registry job must mount its ServiceAccount token")
	}
	if !hasEnv(job.Spec.Template.Spec.Containers[0].Env, "REGISTRY_SERVICE_ACCOUNT_TOKEN_FILE") {
		t.Fatal("local registry job must receive the ServiceAccount token path")
	}
}

func TestExternalRegistryJobDoesNotUseServiceAccountToken(t *testing.T) {
	spec := &BuildImageSpec{BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
		TaskID:             "external-registry",
		Namespace:          "default",
		ServiceAccountName: "founder",
		TargetImage: buildimagev1alpha1.TargetImage{
			Address: "registry.example.com/demo/app:latest",
		},
	}}
	job, err := toBuildJob(context.WithValue(context.Background(), PanelRegistryServerHostKey, "127.0.0.1:8000"), spec)
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Template.Spec.ServiceAccountName != "" {
		t.Fatalf("external job serviceAccountName = %q", job.Spec.Template.Spec.ServiceAccountName)
	}
	if hasEnv(job.Spec.Template.Spec.Containers[0].Env, "REGISTRY_SERVICE_ACCOUNT_TOKEN_FILE") {
		t.Fatal("external registry job must retain its Docker auth-only flow")
	}
}

func hasEnv(envs []corev1.EnvVar, name string) bool {
	for _, env := range envs {
		if env.Name == name {
			return true
		}
	}
	return false
}

func TestToJbo(t *testing.T) {

	spec := &BuildImageSpec{
		BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
			TaskID:    "test1",
			Namespace: "default",
			Source: buildimagev1alpha1.Source{
				DockerfilePath: "Dockerfile",
				// DownloadURL:    "http://118.25.185.46:9090/ui/microapp/ddd3.zip",
				DownloadURL: "http://172.16.1.162:9090/ui/microapp/ddd4.tar.xz",
			},
			TargetImage: buildimagev1alpha1.TargetImage{
				Address: "registry.local.w7.cc/w7panel/test-3:latest",
				Auth: buildimagev1alpha1.Auth{
					Username: "w7panel",
					Password: "w7panel",
				},
			},
		},
	}
	job, err := toBuildJob(context.Background(), spec)
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
