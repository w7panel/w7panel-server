package v1alpha1_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
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

func TestBuiltinBootstrapInstallationsArePruneScoped(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "..")
	manifestPath := filepath.Join(root, "kodata", "yaml", "bootstrap-installations.yaml")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(manifest), 4096)
	count := 0
	for {
		item := &installationv1.BootstrapInstallation{}
		if err := decoder.Decode(item); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatal(err)
		}
		if item.Name == "" {
			continue
		}
		count++
		if item.Labels["w7.cc/bootstrap-builtin"] != "true" {
			t.Fatalf("%s is missing the built-in prune label", item.Name)
		}
	}
	if count == 0 {
		t.Fatal("built-in BootstrapInstallation manifest is empty")
	}

	upgradeScript, err := os.ReadFile(filepath.Join(root, "kodata", "shell", "upgrade.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"--prune",
		"w7.cc/bootstrap-builtin=true",
		"--prune-allowlist='w7panel.w7.com/v1alpha1/BootstrapInstallation'",
	} {
		if !strings.Contains(string(upgradeScript), expected) {
			t.Fatalf("upgrade.sh is missing safe BootstrapInstallation prune option %q", expected)
		}
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
	for _, field := range []string{"strategy", "artifact", "target"} {
		if _, ok := spec.Properties[field]; !ok {
			t.Fatalf("spec.%s is missing", field)
		}
	}
	for _, legacy := range []string{"revision", "profileRef", "profileRevision", "dependsOn", "failurePolicy"} {
		if _, ok := spec.Properties[legacy]; ok {
			t.Fatalf("legacy spec.%s still exists", legacy)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "kodata", "crds", "w7panel.w7.com_bootstrapprofiles.yaml")); !os.IsNotExist(err) {
		t.Fatalf("BootstrapProfile CRD manifest still exists: %v", err)
	}
}
