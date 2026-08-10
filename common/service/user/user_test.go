package user

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFromUnstructuredReadsCloudFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "cloud-user"},
		"spec": map[string]interface{}{
			"cloudId":       "10001",
			"cloudOpenid":   "openid-10001",
			"cloudNickname": "cloud user",
		},
	}}

	u, err := FromUnstructured(obj)
	if err != nil {
		t.Fatalf("FromUnstructured() error = %v", err)
	}
	if u.Spec.CloudId != "10001" {
		t.Fatalf("CloudId = %q, want 10001", u.Spec.CloudId)
	}
	if u.Spec.CloudOpenid != "openid-10001" {
		t.Fatalf("CloudOpenid = %q, want openid-10001", u.Spec.CloudOpenid)
	}
	if u.Spec.CloudNickname != "cloud user" {
		t.Fatalf("CloudNickname = %q, want cloud user", u.Spec.CloudNickname)
	}
}

func TestFromUnstructuredReadsLegacyConsoleFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "legacy-user"},
		"spec": map[string]interface{}{
			"consoleId":       "10002",
			"consoleOpenid":   "openid-10002",
			"consoleNickname": "legacy user",
		},
	}}

	u, err := FromUnstructured(obj)
	if err != nil {
		t.Fatalf("FromUnstructured() error = %v", err)
	}
	if u.Spec.CloudId != "10002" {
		t.Fatalf("CloudId = %q, want 10002", u.Spec.CloudId)
	}
	if u.Spec.CloudOpenid != "openid-10002" {
		t.Fatalf("CloudOpenid = %q, want openid-10002", u.Spec.CloudOpenid)
	}
	if u.Spec.CloudNickname != "legacy user" {
		t.Fatalf("CloudNickname = %q, want legacy user", u.Spec.CloudNickname)
	}
}

func TestToUnstructuredWritesOnlyCloudFields(t *testing.T) {
	obj, err := ToUnstructured("cloud-user", Spec{
		CloudId:       "10003",
		CloudOpenid:   "openid-10003",
		CloudNickname: "new user",
		Cloud: &W7Config{
			ClusterId: "cluster-10003",
		},
	})
	if err != nil {
		t.Fatalf("ToUnstructured() error = %v", err)
	}

	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloudId"); got != "10003" {
		t.Fatalf("spec.cloudId = %q, want 10003", got)
	}
	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloud", "clusterId"); got != "cluster-10003" {
		t.Fatalf("spec.cloud.clusterId = %q, want cluster-10003", got)
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleId"); ok {
		t.Fatal("legacy spec.consoleId should not be written")
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleOpenid"); ok {
		t.Fatal("legacy spec.consoleOpenid should not be written")
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleNickname"); ok {
		t.Fatal("legacy spec.consoleNickname should not be written")
	}
}

func TestMigrateLegacyConsoleFieldsCopiesAndDeletesLegacyFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "legacy-user"},
		"spec": map[string]interface{}{
			"consoleId":       "10004",
			"consoleOpenid":   "openid-10004",
			"consoleNickname": "legacy user",
		},
	}}

	if !migrateLegacyConsoleFields(obj) {
		t.Fatal("migrateLegacyConsoleFields() = false, want true")
	}
	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloudId"); got != "10004" {
		t.Fatalf("spec.cloudId = %q, want 10004", got)
	}
	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloudOpenid"); got != "openid-10004" {
		t.Fatalf("spec.cloudOpenid = %q, want openid-10004", got)
	}
	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloudNickname"); got != "legacy user" {
		t.Fatalf("spec.cloudNickname = %q, want legacy user", got)
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleId"); ok {
		t.Fatal("legacy spec.consoleId should be deleted")
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleOpenid"); ok {
		t.Fatal("legacy spec.consoleOpenid should be deleted")
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleNickname"); ok {
		t.Fatal("legacy spec.consoleNickname should be deleted")
	}
}

func TestMigrateLegacyConsoleFieldsKeepsExistingCloudFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"cloudId":   "new-id",
			"consoleId": "old-id",
		},
	}}

	if !migrateLegacyConsoleFields(obj) {
		t.Fatal("migrateLegacyConsoleFields() = false, want true")
	}
	if got, _, _ := unstructured.NestedString(obj.Object, "spec", "cloudId"); got != "new-id" {
		t.Fatalf("spec.cloudId = %q, want new-id", got)
	}
	if _, ok, _ := unstructured.NestedString(obj.Object, "spec", "consoleId"); ok {
		t.Fatal("legacy spec.consoleId should be deleted")
	}
}
