package permission

import (
	"context"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureMetricsReaderAccess(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rbacv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	controller := &PermissionController{Client: client}

	for range 2 {
		if err := controller.ensureMetricsReaderAccess(context.Background(), NormalPermissionName); err != nil {
			t.Fatalf("ensureMetricsReaderAccess() error = %v", err)
		}
	}

	binding := &rbacv1.ClusterRoleBinding{}
	name := NormalPermissionName + "-ckm-metrics-reader"
	if err := client.Get(context.Background(), types.NamespacedName{Name: name}, binding); err != nil {
		t.Fatalf("get ClusterRoleBinding %s: %v", name, err)
	}
	if binding.RoleRef != (rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "ckm-metrics-reader",
	}) {
		t.Fatalf("role ref = %#v", binding.RoleRef)
	}
	wantSubject := rbacv1.Subject{Kind: "ServiceAccount", Name: NormalPermissionName, Namespace: "default"}
	if len(binding.Subjects) != 1 || binding.Subjects[0] != wantSubject {
		t.Fatalf("subjects = %#v, want %#v", binding.Subjects, wantSubject)
	}
}

func TestEnsureMetricsReaderAccessRequiresClient(t *testing.T) {
	controller := &PermissionController{}
	if err := controller.ensureMetricsReaderAccess(context.Background(), NormalPermissionName); err == nil {
		t.Fatal("ensureMetricsReaderAccess() expected error without client")
	}
}
