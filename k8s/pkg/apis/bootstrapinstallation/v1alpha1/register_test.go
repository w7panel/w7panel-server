package v1alpha1_test

import (
	"os"
	"path/filepath"
	"testing"

	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func TestBootstrapResourcesUseW7PanelAPIGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := installationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	installationGVKs, _, err := scheme.ObjectKinds(&installationv1.BootstrapInstallation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(installationGVKs) != 1 || installationGVKs[0].Group != "w7panel.w7.com" || installationGVKs[0].Version != "v1alpha1" {
		t.Fatalf("unexpected BootstrapInstallation GVKs: %#v", installationGVKs)
	}

	profileGVKs, _, err := scheme.ObjectKinds(&bootstrapv1.BootstrapProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if len(profileGVKs) != 1 || profileGVKs[0].Group != "w7panel.w7.com" || profileGVKs[0].Version != "v1alpha1" {
		t.Fatalf("unexpected BootstrapProfile GVKs: %#v", profileGVKs)
	}
}

func TestBootstrapCRDsUseW7PanelAPIGroup(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	manifestPath := filepath.Join(root, "kodata", "crds", "w7panel.w7.com_bootstrapinstallations.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(manifest, crd); err != nil {
		t.Fatal(err)
	}
	if crd.Name != "bootstrapinstallations.w7panel.w7.com" {
		t.Fatalf("CRD name = %q", crd.Name)
	}
	if crd.Spec.Group != "w7panel.w7.com" {
		t.Fatalf("CRD group = %q", crd.Spec.Group)
	}
	if crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		t.Fatalf("CRD scope = %q", crd.Spec.Scope)
	}
	if crd.Spec.Names.Kind != "BootstrapInstallation" || crd.Spec.Names.Plural != "bootstrapinstallations" {
		t.Fatalf("unexpected BootstrapInstallation names: %#v", crd.Spec.Names)
	}

	legacyPath := filepath.Join(root, "kodata", "crds", "w7panel.w7.com_artifactinstallations.yaml")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy CRD manifest still exists: %v", err)
	}

	profileManifestPath := filepath.Join(root, "kodata", "crds", "w7panel.w7.com_bootstrapprofiles.yaml")
	profileManifest, err := os.ReadFile(profileManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	profileCRD := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(profileManifest, profileCRD); err != nil {
		t.Fatal(err)
	}
	if profileCRD.Name != "bootstrapprofiles.w7panel.w7.com" || profileCRD.Spec.Group != "w7panel.w7.com" {
		t.Fatalf("unexpected BootstrapProfile CRD: name=%q group=%q", profileCRD.Name, profileCRD.Spec.Group)
	}
	profileSpec := profileCRD.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	if _, ok := profileSpec.Properties["installations"]; !ok {
		t.Fatal("BootstrapProfile CRD spec.installations is missing")
	}
	if _, ok := profileSpec.Properties["artifacts"]; ok {
		t.Fatal("BootstrapProfile CRD still exposes spec.artifacts")
	}

	legacyProfilePath := filepath.Join(root, "kodata", "crds", "bootstrap.w7.cc_bootstrapprofiles.yaml")
	if _, err := os.Stat(legacyProfilePath); !os.IsNotExist(err) {
		t.Fatalf("legacy BootstrapProfile CRD manifest still exists: %v", err)
	}
}
