package controller

import (
	"testing"

	helmservice "github.com/w7panel/w7panel/common/service/helm"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestPreparedCleanupNamesBindsSharedChoice(t *testing.T) {
	t.Setenv("PANEL_AUTH_SIGNING_KEY", "test-key")
	group := &appv1.AppGroup{ObjectMeta: metav1.ObjectMeta{
		Namespace: "default", UID: types.UID("uid-1"),
		Annotations: map[string]string{
			helmservice.CRDCleanupAnnotation:            `["widgets.example.com"]`,
			helmservice.CRDCleanupAfterGroupsAnnotation: `[]`,
			helmservice.CRDCleanupForceSharedAnnotation: "false",
		},
	}}
	signature, err := helmservice.CRDCleanupSignature(group.Namespace, string(group.UID), `["widgets.example.com"]`+"\n[]\nfalse")
	if err != nil {
		t.Fatal(err)
	}
	group.Annotations[helmservice.CRDCleanupSignatureAnnotation] = signature
	names, err := preparedCleanupNames(group)
	if err != nil || len(names) != 1 || names[0] != "widgets.example.com" {
		t.Fatalf("valid cleanup intent rejected: %v, %v", names, err)
	}
	group.Annotations[helmservice.CRDCleanupForceSharedAnnotation] = "true"
	if _, err := preparedCleanupNames(group); err == nil {
		t.Fatal("changed shared-resource confirmation was accepted")
	}
}

func TestValidateUninstallDependents(t *testing.T) {
	groups := map[string]*appv1.AppGroup{
		"root":       {},
		"child":      {Spec: appv1.AppGroupSpec{Dependencies: []appv1.AppGroupDependency{{Name: "root"}}}},
		"grandchild": {Spec: appv1.AppGroupSpec{Dependencies: []appv1.AppGroupDependency{{Name: "child", Namespace: "default"}}}},
	}
	if err := validateUninstallDependents(groups, "default", "root"); err != nil {
		t.Fatal(err)
	}
	groups["unrelated"] = &appv1.AppGroup{}
	if err := validateUninstallDependents(groups, "default", "root"); err == nil {
		t.Fatal("unrelated application accepted")
	}
	groups["unrelated"].Spec.Dependencies = []appv1.AppGroupDependency{{Name: "root", Namespace: "other"}}
	if err := validateUninstallDependents(groups, "default", "root"); err == nil {
		t.Fatal("cross-namespace dependency accepted")
	}
}

func TestOmittedUninstallDependent(t *testing.T) {
	all := []appv1.AppGroup{{
		ObjectMeta: metav1.ObjectMeta{Name: "dependent"},
		Spec:       appv1.AppGroupSpec{Dependencies: []appv1.AppGroupDependency{{Name: "root"}}},
	}}
	if got := omittedUninstallDependent(all, map[string]bool{"root": true}, "default"); got != "dependent" {
		t.Fatalf("omitted dependent was not detected: %q", got)
	}
	if got := omittedUninstallDependent(all, map[string]bool{"root": true, "dependent": true}, "default"); got != "" {
		t.Fatalf("selected dependent was rejected: %q", got)
	}
	all[0].Spec.Dependencies[0].Namespace = "other"
	if got := omittedUninstallDependent(all, map[string]bool{"root": true}, "default"); got != "" {
		t.Fatalf("other namespace was treated as dependent: %q", got)
	}
}
