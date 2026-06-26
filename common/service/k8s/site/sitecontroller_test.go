package site

import (
	"context"
	"testing"

	"github.com/w7panel/w7panel/common/service/console"
	appgroupv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	sitev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/site/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPatchTargetResourceAppGroupStoresAppCredentialsInSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appgroupv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add appgroup scheme: %v", err)
	}

	existing := &appgroupv1alpha1.AppGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(existing).Build()
	controller := &SiteController{Client: client}

	err := controller.patchTargetResource(&sitev1alpha1.TargetRef{
		Kind:      "AppGroup",
		Name:      "demo",
		Namespace: "default",
	}, &console.AppSecret{
		AppId:     "app-id",
		AppSecret: "app-secret",
	})
	if err != nil {
		t.Fatalf("patchTargetResource() error = %v", err)
	}

	got := &appgroupv1alpha1.AppGroup{}
	err = client.Get(context.Background(), k8stypes.NamespacedName{Name: "demo", Namespace: "default"}, got)
	if err != nil {
		t.Fatalf("get appgroup: %v", err)
	}
	if got.Spec.AppCredentials == nil {
		t.Fatal("spec.appCredentials is nil")
	}
	if got.Spec.AppCredentials.AppId != "app-id" {
		t.Fatalf("spec.appCredentials.appId = %q, want %q", got.Spec.AppCredentials.AppId, "app-id")
	}
	if got.Spec.AppCredentials.AppSecret != "app-secret" {
		t.Fatalf("spec.appCredentials.appSecret = %q, want %q", got.Spec.AppCredentials.AppSecret, "app-secret")
	}
}
