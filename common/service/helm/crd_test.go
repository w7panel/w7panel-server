package helm

import (
	"context"
	"reflect"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestChangedCRDsOnlyReturnsNewOrChangedDefinitions(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	spec := map[string]interface{}{
		"group": "example.com", "scope": "Namespaced",
		"names": map[string]interface{}{"kind": "Widget", "plural": "widgets"},
	}
	makeCRD := func(name string, definition map[string]interface{}) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
			"metadata": map[string]interface{}{"name": name}, "spec": definition,
		}}
	}
	if _, err := client.Resource(crdResource).Create(context.Background(), makeCRD("same.example.com", spec), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Resource(crdResource).Create(context.Background(), makeCRD("changed.example.com", spec), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	changedSpec := map[string]interface{}{
		"group": "another.example.com", "scope": "Namespaced",
		"names": map[string]interface{}{"kind": "Widget", "plural": "widgets"},
	}
	names, err := changedCRDs(context.Background(), client, map[string]*unstructured.Unstructured{
		"same.example.com":    makeCRD("same.example.com", spec),
		"changed.example.com": makeCRD("changed.example.com", changedSpec),
		"new.example.com":     makeCRD("new.example.com", spec),
	})
	if err != nil || !reflect.DeepEqual(names, []string{"changed.example.com", "new.example.com"}) {
		t.Fatalf("unexpected changed CRDs: %v, %v", names, err)
	}
}

func TestChartCRDsAndCleanupSignature(t *testing.T) {
	if _, err := ChartDigest(nil); err == nil {
		t.Fatal("missing Chart accepted")
	}
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "demo", Version: "1.0.0"}, Files: []*chart.File{
		{Name: "crds/widgets.yaml", Data: []byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata:\n  name: widgets.example.com\nspec:\n  group: example.com\n")},
		{Name: "templates/ignored.yaml", Data: []byte("invalid")},
	}}
	crds, err := ChartCRDs(ch)
	if err != nil || len(crds) != 1 || crds["widgets.example.com"] == nil {
		t.Fatalf("expected one standard CRD, got %v: %v", crds, err)
	}
	digest, err := ChartDigest(ch)
	if err != nil || digest == "" {
		t.Fatalf("expected Chart digest: %v", err)
	}
	ch.Templates = []*chart.File{{Name: "templates/pod.yaml", Data: []byte("changed")}}
	changedDigest, err := ChartDigest(ch)
	if err != nil || changedDigest == digest {
		t.Fatal("Chart digest did not change with template content")
	}
	desiredSpec := map[string]interface{}{"group": "example.com", "names": map[string]interface{}{"kind": "Widget", "plural": "widgets"}}
	liveSpec := map[string]interface{}{"group": "example.com", "names": map[string]interface{}{"kind": "Widget", "plural": "widgets", "singular": "widget", "listKind": "WidgetList"}, "conversion": map[string]interface{}{"strategy": "None"}}
	if !equalCRDSpec(desiredSpec, liveSpec) {
		t.Fatal("Kubernetes defaulted CRD fields should not trigger an update")
	}
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test-key")
	intent := `["widgets.example.com"]` + "\n" + `["dependency"]` + "\nfalse"
	signature, err := CRDCleanupSignature("default", "uid-1", intent)
	if err != nil || !VerifyCRDCleanupSignature("default", "uid-1", intent, signature) {
		t.Fatalf("valid cleanup authorization was rejected: %v", err)
	}
	if VerifyCRDCleanupSignature("default", "uid-2", intent, signature) ||
		VerifyCRDCleanupSignature("default", "uid-1", `["secrets.example.com"]`+"\n"+`["dependency"]`+"\nfalse", signature) ||
		VerifyCRDCleanupSignature("default", "uid-1", `["widgets.example.com"]`+"\n"+`[]`+"\nfalse", signature) ||
		VerifyCRDCleanupSignature("default", "uid-1", `["widgets.example.com"]`+"\n"+`["dependency"]`+"\ntrue", signature) {
		t.Fatal("tampered cleanup authorization was accepted")
	}
}

func TestDeleteCRDsWaitsUntilGone(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	name := "widgets.example.com"
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]interface{}{"name": name},
	}}
	if _, err := client.Resource(crdResource).Create(context.Background(), crd, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	client.PrependReactor("delete", crdResource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.DeleteAction).GetName() == name {
			return true, nil, nil // The API accepted deletion, but finalizers still hold the CRD.
		}
		return false, nil, nil
	})
	gets := 0
	client.PrependReactor("get", crdResource.Resource, func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.(k8stesting.GetAction).GetName() == name {
			gets++
			if gets == 2 {
				if err := client.Tracker().Delete(crdResource, "", name); err != nil {
					return true, nil, err
				}
			}
		}
		return false, nil, nil
	})
	if err := deleteCRDs(context.Background(), client, []string{name, "already-gone.example.com"}); err != nil {
		t.Fatal(err)
	}
	if gets < 2 {
		t.Fatalf("deletion returned before CRD disappeared: %d gets", gets)
	}
	if _, err := client.Resource(crdResource).Get(context.Background(), name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("CRD still exists after cleanup: %v", err)
	}
}

func TestCustomResourceOwnershipChecksNamespace(t *testing.T) {
	item := unstructured.Unstructured{}
	item.SetLabels(map[string]string{"w7.cc/group-name": "app"})
	item.SetNamespace("other")
	if customResourceOwnedByRelease(item, "app", "default") {
		t.Fatal("resource in another namespace treated as owned")
	}
	item.SetNamespace("default")
	if !customResourceOwnedByRelease(item, "app", "default") {
		t.Fatal("matching resource treated as shared")
	}
	item.SetNamespace("")
	if customResourceOwnedByRelease(item, "app", "default") {
		t.Fatal("cluster-scoped resource treated as provably owned")
	}
}

func TestCustomResourceRisksUseLiveCRDVersion(t *testing.T) {
	resource := schema.GroupVersionResource{Group: "example.com", Version: "v2", Resource: "widgets"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		resource: "WidgetList",
	})
	name := "widgets.example.com"
	crd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
		"metadata": map[string]interface{}{"name": name},
		"spec": map[string]interface{}{
			"group":    "example.com",
			"names":    map[string]interface{}{"plural": "widgets"},
			"versions": []interface{}{map[string]interface{}{"name": "v2", "served": true}},
		},
	}}
	if _, err := client.Resource(crdResource).Create(context.Background(), crd, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	foreign := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v2", "kind": "Widget",
		"metadata": map[string]interface{}{"name": "foreign", "namespace": "other"},
	}}
	if _, err := client.Resource(resource).Namespace("other").Create(context.Background(), foreign, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	risks := customResourceRisks(context.Background(), client, []string{name}, "app", "default")
	if len(risks) != 1 {
		t.Fatalf("live v2 resource was missed: %v", risks)
	}
}

func TestApplyCRDsCreatesAndUpdates(t *testing.T) {
	client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	makeCRD := func(name, group string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1", "kind": "CustomResourceDefinition",
			"metadata": map[string]interface{}{"name": name},
			"spec":     map[string]interface{}{"group": group},
			"status": map[string]interface{}{"conditions": []interface{}{
				map[string]interface{}{"type": "Established", "status": "True"},
			}},
		}}
	}
	if _, err := client.Resource(crdResource).Create(context.Background(), makeCRD("existing.example.com", "old.example.com"), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := applyCRDs(context.Background(), client, map[string]*unstructured.Unstructured{
		"existing.example.com": makeCRD("existing.example.com", "new.example.com"),
		"new.example.com":      makeCRD("new.example.com", "new.example.com"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"existing.example.com", "new.example.com"} {
		crd, err := client.Resource(crdResource).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil || crd.Object["spec"].(map[string]interface{})["group"] != "new.example.com" {
			t.Fatalf("CRD %s not applied: %v, %v", name, crd, err)
		}
	}
}
