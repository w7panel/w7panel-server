package coredns

import (
	"context"
	"os"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLoadConfig(t *testing.T) {
	if os.Getenv("COREDNS_LIVE_TEST") != "true" {
		t.Skip("set COREDNS_LIVE_TEST=true to read live kube-system/coredns-custom")
	}
	json, err := ParseToJsonConfig()
	if err != nil {
		t.Error(err)
		return
	}
	jsonstr := string(json)
	t.Log(jsonstr)
}

func TestEnsureCoreDNSCustomImport(t *testing.T) {
	corefile := ".:53 {\n  errors\n}\n"
	next := ensureCoreDNSCustomImport(corefile)
	expected := ".:53 {\n  errors\n}\nimport /etc/coredns/custom/*.server\n"
	if next != expected {
		t.Fatalf("unexpected corefile:\n%s", next)
	}
	if again := ensureCoreDNSCustomImport(next); again != next {
		t.Fatalf("expected import to be idempotent, got:\n%s", again)
	}
}

func TestEnsureCoreDNSDeploymentCustomConfig(t *testing.T) {
	deployment := coreDNSDeployment()
	ensureCoreDNSDeploymentCustomConfig(deployment)
	ensureCoreDNSDeploymentCustomConfig(deployment)

	if len(deployment.Spec.Template.Spec.Volumes) != 1 {
		t.Fatalf("expected one custom volume, got %d", len(deployment.Spec.Template.Spec.Volumes))
	}
	volume := deployment.Spec.Template.Spec.Volumes[0]
	if volume.Name != coreDNSCustomVolumeName || volume.ConfigMap == nil || volume.ConfigMap.Name != CoreDNSCustomName {
		t.Fatalf("unexpected custom volume: %#v", volume)
	}
	if volume.ConfigMap.Optional == nil || !*volume.ConfigMap.Optional {
		t.Fatalf("expected custom configmap volume to be optional")
	}
	mounts := deployment.Spec.Template.Spec.Containers[0].VolumeMounts
	if len(mounts) != 1 {
		t.Fatalf("expected one custom mount, got %d", len(mounts))
	}
	if mounts[0].Name != coreDNSCustomVolumeName || mounts[0].MountPath != coreDNSCustomMountPath || !mounts[0].ReadOnly {
		t.Fatalf("unexpected custom mount: %#v", mounts[0])
	}
}

func TestUpdateRecordAppliesCoreDNSChange(t *testing.T) {
	ctx := context.Background()
	customZone, err := RenderZone("example.com", []Record{{ID: "record-1", Name: "www", Type: "A", Value: "1.1.1.1", TTL: 60}})
	if err != nil {
		t.Fatal(err)
	}
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSCustomName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{ConfigMapKey("example.com"): customZone},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{coreDNSCorefileKey: ".:53 {\n  errors\n}\n"},
		},
		coreDNSDeployment(),
	)
	service := newServiceWithClient(client)

	updated, err := service.UpdateRecord(ctx, "example.com", "record-1", Record{Name: "www", Type: "A", Value: "2.2.2.2", TTL: 60})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Value != "2.2.2.2" {
		t.Fatalf("unexpected updated record: %#v", updated)
	}
	corefileConfig, err := client.CoreV1().ConfigMaps(CoreDNSNamespace).Get(ctx, CoreDNSName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if corefileConfig.Data[coreDNSCorefileKey] != ".:53 {\n  errors\n}\nimport /etc/coredns/custom/*.server\n" {
		t.Fatalf("unexpected corefile:\n%s", corefileConfig.Data[coreDNSCorefileKey])
	}
	deployment, err := client.AppsV1().Deployments(CoreDNSNamespace).Get(ctx, CoreDNSName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if deployment.Spec.Template.Annotations[coreDNSRestartAnnotationKey] == "" {
		t.Fatalf("expected restart annotation")
	}
	if !hasCoreDNSCustomVolume(deployment.Spec.Template.Spec.Volumes) {
		t.Fatalf("expected custom config volume")
	}
	if !hasCoreDNSCustomVolumeMount(deployment.Spec.Template.Spec.Containers[0].VolumeMounts) {
		t.Fatalf("expected custom config mount")
	}
}

func coreDNSDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      CoreDNSName,
			Namespace: CoreDNSNamespace,
			Labels:    map[string]string{"k8s-app": "kube-dns"},
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "coredns"}},
				},
			},
		},
	}
}
