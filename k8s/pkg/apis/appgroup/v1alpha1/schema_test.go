package v1alpha1

import (
	"os"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

func TestAppGroupStatusOptionalForVeleroRestore(t *testing.T) {
	data, err := os.ReadFile("../../../../../kodata/crds/w7panel.w7.com_appgroups.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var crd apiextensionsv1.CustomResourceDefinition
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	schema := crd.Spec.Versions[0].Schema.OpenAPIV3Schema
	for _, required := range schema.Required {
		if required == "status" {
			t.Fatal("AppGroup status must be optional so Velero can create a restored object")
		}
	}
	if _, ok := schema.Properties["status"]; !ok {
		t.Fatal("AppGroup status schema must remain defined")
	}
}
