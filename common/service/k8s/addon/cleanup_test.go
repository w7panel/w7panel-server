package addon

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestCleanupRemovesLegacySourcesAndObjects(t *testing.T) {
	hostRoot := t.TempDir()
	manifestDir := filepath.Join(hostRoot, manifestsRelativePath)
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range legacyManifestFiles {
		if err := os.WriteFile(filepath.Join(manifestDir, name), []byte("legacy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	for _, name := range legacyAddons {
		if _, err := client.Resource(addonGVR).Create(context.Background(), object(addonGVR, "", name), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.Resource(helmChartGVR).Namespace("kube-system").Create(context.Background(), object(helmChartGVR, "kube-system", "higress"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(context.Background(), client, hostRoot); err != nil {
		t.Fatal(err)
	}
	for _, name := range legacyManifestFiles {
		if _, err := os.Stat(filepath.Join(manifestDir, name)); !os.IsNotExist(err) {
			t.Fatalf("manifest %s still exists, err=%v", name, err)
		}
	}
	for _, name := range legacyAddons {
		if _, err := client.Resource(addonGVR).Get(context.Background(), name, metav1.GetOptions{}); err == nil {
			t.Fatalf("addon %s still exists", name)
		}
	}
	if _, err := client.Resource(helmChartGVR).Namespace("kube-system").Get(context.Background(), "higress", metav1.GetOptions{}); err == nil {
		t.Fatal("higress helmchart still exists")
	}
}

func TestCleanupSkipsWorkerNode(t *testing.T) {
	hostRoot := t.TempDir()
	client := fake.NewSimpleDynamicClient(runtime.NewScheme())
	if _, err := client.Resource(addonGVR).Create(context.Background(), object(addonGVR, "", "higress"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := Cleanup(context.Background(), client, hostRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(addonGVR).Get(context.Background(), "higress", metav1.GetOptions{}); err != nil {
		t.Fatalf("worker cleanup must not delete addon: %v", err)
	}
}

func object(gvr schema.GroupVersionResource, namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": gvr.GroupVersion().String(),
		"kind":       "Test",
		"metadata":   map[string]interface{}{"namespace": namespace, "name": name},
	}}
}
