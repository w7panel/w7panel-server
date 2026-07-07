package config

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestUserConfigToW7ConfigReadsCloudMap(t *testing.T) {
	config, err := userConfigToW7Config("console-75780", map[string]interface{}{
		"clusterId": "test-cluster-id",
	})
	if err != nil {
		t.Fatalf("userConfigToW7Config() error = %v", err)
	}
	if config.ClusterId != "test-cluster-id" {
		t.Fatalf("ClusterId = %v, want test-cluster-id", config.ClusterId)
	}
}

func TestNestedUserCloudConfigUsesCloudField(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"cloud": map[string]interface{}{
					"clusterId": "test-cluster-id",
				},
			},
		},
	}

	config, ok, err := nestedUserCloudConfig(obj)
	if err != nil {
		t.Fatalf("nestedUserCloudConfig() error = %v", err)
	}
	if !ok {
		t.Fatal("nestedUserCloudConfig() ok = false, want true")
	}
	if config["clusterId"] != "test-cluster-id" {
		t.Fatalf("clusterId = %v, want test-cluster-id", config["clusterId"])
	}
}

func TestNestedUserCloudConfigDoesNotReadLegacyW7Config(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"w7Config": map[string]interface{}{
					"clusterId": "legacy-cluster-id",
				},
			},
		},
	}

	_, ok, err := nestedUserCloudConfig(obj)
	if err != nil {
		t.Fatalf("nestedUserCloudConfig() error = %v", err)
	}
	if ok {
		t.Fatal("nestedUserCloudConfig() read legacy w7Config, want only cloud")
	}
}

func TestSetNestedUserCloudConfigWritesOnlyCloudField(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}

	err := setNestedUserCloudConfig(obj, map[string]interface{}{
		"clusterId": "test-cluster-id",
	})
	if err != nil {
		t.Fatalf("setNestedUserCloudConfig() error = %v", err)
	}

	cloud, ok, err := unstructured.NestedMap(obj.Object, "spec", "cloud")
	if err != nil {
		t.Fatalf("NestedMap(spec.cloud) error = %v", err)
	}
	if !ok {
		t.Fatal("spec.cloud not found")
	}
	if cloud["clusterId"] != "test-cluster-id" {
		t.Fatalf("clusterId = %v, want test-cluster-id", cloud["clusterId"])
	}
	if _, ok, _ := unstructured.NestedMap(obj.Object, "spec", "w7Config"); ok {
		t.Fatal("legacy spec.w7Config should not be written")
	}
}

func TestUserConfigToW7ConfigMissingCloudError(t *testing.T) {
	_, err := userConfigToW7Config("admin", nil)
	if err == nil {
		t.Fatal("expected error for missing cloud config")
	}
	if !strings.Contains(err.Error(), "cloud config") {
		t.Fatalf("error = %q, want cloud config", err.Error())
	}
}
