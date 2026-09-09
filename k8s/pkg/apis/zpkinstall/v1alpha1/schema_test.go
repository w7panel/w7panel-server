package v1alpha1

import (
	"context"
	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	validation "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/validation"
	structural "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	schemacel "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"os"
	"sigs.k8s.io/yaml"
	"testing"
)

func TestZpkInstallCRDSchema(t *testing.T) {
	data, err := os.ReadFile("../../../../../kodata/crds/w7panel.w7.com_zpkinstalls.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var crd extv1.CustomResourceDefinition
	if err = yaml.Unmarshal(data, &crd); err != nil {
		t.Fatal(err)
	}
	var internal apiext.CustomResourceDefinition
	if err = extv1.Convert_v1_CustomResourceDefinition_To_apiextensions_CustomResourceDefinition(&crd, &internal, nil); err != nil {
		t.Fatal(err)
	}
	// The API server defaults this field before validating the CRD.
	internal.Spec.Conversion = &apiext.CustomResourceConversion{Strategy: apiext.NoneConverter}
	internal.Status.StoredVersions = []string{"v1alpha1"}
	if errs := validation.ValidateCustomResourceDefinition(context.Background(), &internal); len(errs) > 0 {
		t.Fatal(errs)
	}
	root, err := structural.NewStructural(internal.Spec.Validation.OpenAPIV3Schema)
	if err != nil {
		t.Fatal(err)
	}
	validator := schemacel.NewValidator(root, true, 10000000)
	original := map[string]interface{}{"apiVersion": "w7panel.w7.com/v1alpha1", "kind": "ZpkInstall", "metadata": map[string]interface{}{"name": "app"}, "spec": map[string]interface{}{"repoUrl": "https://example.com/one", "releaseName": "app", "installOptions": []interface{}{}}}
	if errs, _ := validator.Validate(context.Background(), field.NewPath(""), root, original, nil, 10000000); len(errs) != 0 {
		t.Fatalf("create rejected: %v", errs)
	}
	if errs, _ := validator.Validate(context.Background(), field.NewPath(""), root, original, original, 10000000); len(errs) != 0 {
		t.Fatalf("unchanged spec rejected: %v", errs)
	}
	changed := map[string]interface{}{"apiVersion": "w7panel.w7.com/v1alpha1", "kind": "ZpkInstall", "metadata": map[string]interface{}{"name": "app"}, "spec": map[string]interface{}{"repoUrl": "https://example.com/two", "releaseName": "app", "installOptions": []interface{}{}}}
	if errs, _ := validator.Validate(context.Background(), field.NewPath(""), root, changed, original, 10000000); len(errs) == 0 {
		t.Fatal("spec change accepted")
	}
	spec := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	if len(spec.XValidations) != 1 || spec.XValidations[0].Rule != "self == oldSelf" {
		t.Fatal("immutable spec validation is missing")
	}
	if retries := spec.Properties["maxRetries"]; retries.Type != "integer" || retries.Format != "int32" || retries.Minimum == nil || *retries.Minimum != 0 {
		t.Fatalf("maxRetries schema is missing or invalid: %#v", retries)
	}
	status := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	if retries := status.Properties["retryCount"]; retries.Type != "integer" || retries.Format != "int32" || retries.Minimum == nil || *retries.Minimum != 0 {
		t.Fatalf("retryCount schema is missing or invalid: %#v", retries)
	}
	option := spec.Properties["installOptions"].Items.Schema
	for _, field := range []string{"K8sToken", "RealToken", "IsChild", "serviceAccountName", "buildImageSuccessUrl", "installId"} {
		if _, ok := option.Properties[field]; ok {
			t.Fatalf("internal field exposed: %s", field)
		}
	}
}
