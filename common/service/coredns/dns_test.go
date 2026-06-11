package coredns

import (
	"context"
	"os"
	"strings"
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
	original, err := NormalizeRecord("example.com", Record{Name: "www", Type: "A", Value: "1.1.1.1", TTL: 60})
	if err != nil {
		t.Fatal(err)
	}
	customServer, err := RenderZoneServer("example.com")
	if err != nil {
		t.Fatal(err)
	}
	customZone, err := RenderZone("example.com", []Record{original})
	if err != nil {
		t.Fatal(err)
	}
	originalSerial := extractZoneSerial(customZone)
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSCustomName, Namespace: CoreDNSNamespace},
			Data: map[string]string{
				ConfigMapKey("example.com"):         customServer,
				ZoneFileConfigMapKey("example.com"): customZone,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{coreDNSCorefileKey: ".:53 {\n  errors\n}\n"},
		},
		coreDNSDeployment(),
	)
	service := newServiceWithClient(client)

	updated, err := service.UpdateRecord(ctx, "example.com", original.ID, Record{Name: "www", Type: "A", Value: "2.2.2.2", TTL: 60})
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
	customConfig, err := client.CoreV1().ConfigMaps(CoreDNSNamespace).Get(ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if customConfig.Data[ConfigMapKey("example.com")] != customServer {
		t.Fatalf("unexpected server block:\n%s", customConfig.Data[ConfigMapKey("example.com")])
	}
	zoneData := customConfig.Data[ZoneFileConfigMapKey("example.com")]
	if !strings.Contains(zoneData, "www 60 IN A 2.2.2.2") {
		t.Fatalf("expected updated zone record, got:\n%s", zoneData)
	}
	if serial := extractZoneSerial(zoneData); serial <= originalSerial {
		t.Fatalf("expected serial to increase from %d, got %d", originalSerial, serial)
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

func TestCreateZoneWritesServerAndZoneFiles(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSCustomName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{coreDNSCorefileKey: ".:53 {\n  errors\n}\n"},
		},
		coreDNSDeployment(),
	)
	service := newServiceWithClient(client)

	if _, err := service.CreateZone(ctx, "test4.com"); err != nil {
		t.Fatal(err)
	}
	customConfig, err := client.CoreV1().ConfigMaps(CoreDNSNamespace).Get(ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(customConfig.Data[ConfigMapKey("test4.com")], "file /etc/coredns/custom/test4.com.zone") {
		t.Fatalf("expected server block to reference zone file, got:\n%s", customConfig.Data[ConfigMapKey("test4.com")])
	}
	if !strings.Contains(customConfig.Data[ZoneFileConfigMapKey("test4.com")], "$ORIGIN test4.com.") {
		t.Fatalf("expected zone file, got:\n%s", customConfig.Data[ZoneFileConfigMapKey("test4.com")])
	}
}

func TestListRecordsMigratesLegacyTemplateZone(t *testing.T) {
	ctx := context.Background()
	legacyServer := `test4.com {
  template IN A test4.com. {
    answer "test4.com. 60 IN A 8.8.8.8"
  }

  template IN A a.test4.com. {
    answer "a.test4.com. 1 IN A 10.42.0.154"
  }

  template ANY ANY {
    rcode NOERROR
  }

  loadbalance
}
`
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSCustomName, Namespace: CoreDNSNamespace},
			Data: map[string]string{
				ConfigMapKey("test4.com"): legacyServer,
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{coreDNSCorefileKey: ".:53 {\n  errors\n}\n"},
		},
		coreDNSDeployment(),
	)
	service := newServiceWithClient(client)

	records, err := service.ListRecords(ctx, "test4.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 migrated records, got %d: %#v", len(records), records)
	}
	customConfig, err := client.CoreV1().ConfigMaps(CoreDNSNamespace).Get(ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	serverData := customConfig.Data[ConfigMapKey("test4.com")]
	if strings.Contains(serverData, "template ") {
		t.Fatalf("expected template server to be replaced, got:\n%s", serverData)
	}
	if !strings.Contains(serverData, "file /etc/coredns/custom/test4.com.zone") {
		t.Fatalf("expected migrated server block to reference zone file, got:\n%s", serverData)
	}
	zoneData := customConfig.Data[ZoneFileConfigMapKey("test4.com")]
	if !strings.Contains(zoneData, "$ORIGIN test4.com.") {
		t.Fatalf("expected migrated zone origin, got:\n%s", zoneData)
	}
	if !strings.Contains(zoneData, "@ 60 IN A 8.8.8.8") || !strings.Contains(zoneData, "a 1 IN A 10.42.0.154") {
		t.Fatalf("expected migrated records, got:\n%s", zoneData)
	}
}

func TestDeleteZoneRemovesServerAndZoneFiles(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSCustomName, Namespace: CoreDNSNamespace},
			Data: map[string]string{
				ConfigMapKey("example.com"):         "server",
				ZoneFileConfigMapKey("example.com"): "zone",
			},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: CoreDNSName, Namespace: CoreDNSNamespace},
			Data:       map[string]string{coreDNSCorefileKey: ".:53 {\n  errors\n}\n"},
		},
		coreDNSDeployment(),
	)
	service := newServiceWithClient(client)

	if err := service.DeleteZone(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	customConfig, err := client.CoreV1().ConfigMaps(CoreDNSNamespace).Get(ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := customConfig.Data[ConfigMapKey("example.com")]; ok {
		t.Fatalf("expected server key to be deleted")
	}
	if _, ok := customConfig.Data[ZoneFileConfigMapKey("example.com")]; ok {
		t.Fatalf("expected zone key to be deleted")
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
