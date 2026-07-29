package site

import (
	"context"
	"errors"
	"testing"

	"github.com/w7corp/sdk-open-cloud-go/service"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	appgroupv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	sitev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/site/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSiteUserOpenID(t *testing.T) {
	tests := []struct {
		name      string
		w7config  *config.W7Config
		configErr error
		want      string
		wantErr   string
	}{
		{name: "config error", configErr: errors.New("not found"), wantErr: "get user cloud config: not found"},
		{name: "missing config", wantErr: "user cloud config is empty"},
		{name: "missing user info", w7config: &config.W7Config{}, wantErr: "user info is empty"},
		{name: "missing openid", w7config: &config.W7Config{UserInfo: &service.ResultUserinfo{}}, wantErr: "user OpenID is empty"},
		{name: "success", w7config: &config.W7Config{UserInfo: &service.ResultUserinfo{OpenId: "openid"}}, want: "openid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := siteUserOpenID(tt.w7config, tt.configErr)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("siteUserOpenID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("OpenID = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestSiteIdentifierChangeResetsRegistration(t *testing.T) {
	registeredAt := metav1.Now()
	site := &sitev1alpha1.Site{
		Spec: sitev1alpha1.SiteSpec{SiteIdentifier: "new-identifier"},
		Status: sitev1alpha1.SiteStatus{
			Phase:                  "Completed",
			AppId:                  "old-app-id",
			AppSecret:              "old-app-secret",
			ObservedSiteIdentifier: "old-identifier",
			LastRegisteredAt:       &registeredAt,
			RegisterRetryCount:     2,
			PatchRetryCount:        1,
		},
	}

	if !siteIdentifierChanged(site) {
		t.Fatal("siteIdentifierChanged() = false, want true")
	}

	resetRegistrationForSiteIdentifier(site)

	if site.Status.Phase != "Pending" {
		t.Fatalf("phase = %q, want Pending", site.Status.Phase)
	}
	if site.Status.ObservedSiteIdentifier != "new-identifier" {
		t.Fatalf("observedSiteIdentifier = %q, want new-identifier", site.Status.ObservedSiteIdentifier)
	}
	if site.Status.AppId != "" || site.Status.AppSecret != "" {
		t.Fatalf("credentials = (%q, %q), want empty", site.Status.AppId, site.Status.AppSecret)
	}
	if site.Status.LastRegisteredAt != nil {
		t.Fatal("lastRegisteredAt should be cleared")
	}
	if site.Status.RegisterRetryCount != 0 || site.Status.PatchRetryCount != 0 {
		t.Fatalf("retry counts = (%d, %d), want (0, 0)", site.Status.RegisterRetryCount, site.Status.PatchRetryCount)
	}
}

func TestLegacyTerminalSiteWithoutObservedIdentifierReregisters(t *testing.T) {
	site := &sitev1alpha1.Site{
		Spec:   sitev1alpha1.SiteSpec{SiteIdentifier: "identifier"},
		Status: sitev1alpha1.SiteStatus{Phase: "Failed"},
	}

	if !siteIdentifierChanged(site) {
		t.Fatal("legacy terminal Site should be re-registered")
	}
}

func TestUnchangedSiteIdentifierDoesNotReregister(t *testing.T) {
	site := &sitev1alpha1.Site{
		Spec: sitev1alpha1.SiteSpec{
			Host:           "updated.example.com",
			SiteIdentifier: "identifier",
		},
		Status: sitev1alpha1.SiteStatus{
			Phase:                  "Completed",
			ObservedSiteIdentifier: "identifier",
		},
	}

	if siteIdentifierChanged(site) {
		t.Fatal("unchanged SiteIdentifier should not trigger registration")
	}
}
