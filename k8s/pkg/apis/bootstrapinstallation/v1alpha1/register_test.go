package v1alpha1_test

import (
	"os"
	"path/filepath"
	"testing"

	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func TestBootstrapInstallationUsesW7PanelAPIGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := installationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	gvks, _, err := scheme.ObjectKinds(&installationv1.BootstrapInstallation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(gvks) != 1 || gvks[0].Group != "w7panel.w7.com" || gvks[0].Version != "v1alpha1" {
		t.Fatalf("unexpected GVKs: %#v", gvks)
	}
}

func TestBootstrapInstallationCRDIsSelfContained(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	manifest, err := os.ReadFile(filepath.Join(root, "kodata", "crds", "w7panel.w7.com_bootstrapinstallations.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(manifest, crd); err != nil {
		t.Fatal(err)
	}
	if crd.Name != "bootstrapinstallations.w7panel.w7.com" || crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		t.Fatalf("unexpected CRD: %s %s", crd.Name, crd.Spec.Scope)
	}
	spec := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	for _, field := range []string{"revision", "strategy", "artifact", "target"} {
		if _, ok := spec.Properties[field]; !ok {
			t.Fatalf("spec.%s is missing", field)
		}
	}
	for _, legacy := range []string{"profileRef", "profileRevision", "dependsOn", "failurePolicy"} {
		if _, ok := spec.Properties[legacy]; ok {
			t.Fatalf("legacy spec.%s still exists", legacy)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "kodata", "crds", "w7panel.w7.com_bootstrapprofiles.yaml")); !os.IsNotExist(err) {
		t.Fatalf("BootstrapProfile CRD manifest still exists: %v", err)
	}
}
