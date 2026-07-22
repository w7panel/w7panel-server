package v1alpha1_test

import (
	"os"
	"path/filepath"
	"testing"

	artifactv1 "github.com/w7panel/w7panel/k8s/pkg/apis/artifactinstallation/v1alpha1"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

func TestBootstrapResourcesUseW7PanelAPIGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := artifactv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	artifactGVKs, _, err := scheme.ObjectKinds(&artifactv1.ArtifactInstallation{})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifactGVKs) != 1 || artifactGVKs[0].Group != "w7panel.w7.com" || artifactGVKs[0].Version != "v1alpha1" {
		t.Fatalf("unexpected ArtifactInstallation GVKs: %#v", artifactGVKs)
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
	manifestPath := filepath.Join(root, "kodata", "crds", "w7panel.w7.com_artifactinstallations.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal(manifest, crd); err != nil {
		t.Fatal(err)
	}
	if crd.Name != "artifactinstallations.w7panel.w7.com" {
		t.Fatalf("CRD name = %q", crd.Name)
	}
	if crd.Spec.Group != "w7panel.w7.com" {
		t.Fatalf("CRD group = %q", crd.Spec.Group)
	}
	if crd.Spec.Scope != apiextensionsv1.ClusterScoped {
		t.Fatalf("CRD scope = %q", crd.Spec.Scope)
	}

	legacyPath := filepath.Join(root, "kodata", "crds", "bootstrap.w7.cc_artifactinstallations.yaml")
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

	legacyProfilePath := filepath.Join(root, "kodata", "crds", "bootstrap.w7.cc_bootstrapprofiles.yaml")
	if _, err := os.Stat(legacyProfilePath); !os.IsNotExist(err) {
		t.Fatalf("legacy BootstrapProfile CRD manifest still exists: %v", err)
	}
}
