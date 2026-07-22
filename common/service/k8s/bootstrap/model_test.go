package bootstrap

import (
	"strings"
	"testing"

	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestValidateProfile(t *testing.T) {
	t.Setenv("BOOTSTRAP_ALLOWED_SOURCE_HOSTS", "packages.example.com")
	base := bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile"},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision: "1.0.0-1",
			Installations: []bootstrapv1.BootstrapInstallationTemplate{
				{Name: "domain", Identifie: "domain", Source: "https://packages.example.com/info/domain", ReleaseName: "domain", Namespace: "default"},
				{Name: "rate", Identifie: "rate", Source: "https://packages.example.com/info/rate", ReleaseName: "rate", Namespace: "default", DependsOn: []string{"domain"}},
			},
		},
	}

	tests := []struct {
		name    string
		mutate  func(*bootstrapv1.BootstrapProfile)
		wantErr string
	}{
		{name: "valid"},
		{name: "duplicate target", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[1].ReleaseName = "domain"
		}, wantErr: "相同安装目标"},
		{name: "missing dependency", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[1].DependsOn = []string{"missing"}
		}, wantErr: "不存在的依赖"},
		{name: "dependency cycle", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].DependsOn = []string{"rate"}
		}, wantErr: "依赖存在环"},
		{name: "http rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].Source = "http://packages.example.com/info/domain"
		}, wantErr: "仅允许 HTTPS"},
		{name: "oci rejected until executor is available", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].Source = "oci://packages.example.com/domain"
		}, wantErr: "仅允许 HTTPS"},
		{name: "credentials in source rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].Source = "https://user:password@packages.example.com/info/domain"
		}, wantErr: "不能包含用户名或密码"},
		{name: "reserved helm type rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].Type = "Helm"
		}, wantErr: "仅 ZPK 执行器已启用"},
		{name: "invalid helm value rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].InstallOptions.HelmValues = map[string]string{"service.type": "value,with,commas"}
		}, wantErr: "helmValues"},
		{name: "invalid annotation name rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].InstallOptions.Annotations = map[string]string{"invalid key": "true"}
		}, wantErr: "annotations"},
		{name: "internal owner annotation rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Installations[0].InstallOptions.Annotations = map[string]string{bootstrapv1.AnnotationInstallationOwner: "other"}
		}, wantErr: "内部维护"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := base.DeepCopy()
			if test.mutate != nil {
				test.mutate(profile)
			}
			err := validateProfile(profile)
			if test.wantErr == "" && err != nil {
				t.Fatalf("validateProfile() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("validateProfile() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestProfileSettingsMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries *int32
		want       int32
	}{
		{name: "omitted uses default", want: defaultMaxRetries},
		{name: "explicit zero disables retries", maxRetries: ptr.To[int32](0), want: 0},
		{name: "explicit value", maxRetries: ptr.To[int32](5), want: 5},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := &bootstrapv1.BootstrapProfile{
				Spec: bootstrapv1.BootstrapProfileSpec{
					Strategy: bootstrapv1.BootstrapStrategy{MaxRetries: test.maxRetries},
				},
			}
			if got := profileSettings(profile).MaxRetries; got != test.want {
				t.Fatalf("MaxRetries = %d, want %d", got, test.want)
			}
		})
	}
}

func TestEffectiveArtifactDefaultsTypeAndCopiesHelmValues(t *testing.T) {
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile"},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1"},
	}
	artifact := bootstrapv1.BootstrapInstallationTemplate{
		Name: "domain", Identifie: "domain", Source: "https://zpk.w7.cc/domain",
		ReleaseName: "domain", Namespace: "default",
		InstallOptions: bootstrapv1.BootstrapInstallOptions{
			HelmValues:  map[string]string{"service.type": "ClusterIP"},
			Annotations: map[string]string{"w7.cc/deny-delete": "true"},
		},
	}

	spec := effectiveArtifact(profile, artifact)
	if spec.Artifact.Type != bootstrapv1.ArtifactTypeZPK {
		t.Fatalf("artifact type = %q, want %q", spec.Artifact.Type, bootstrapv1.ArtifactTypeZPK)
	}
	spec.InstallOptions.HelmValues["service.type"] = "LoadBalancer"
	if artifact.InstallOptions.HelmValues["service.type"] != "ClusterIP" {
		t.Fatal("effectiveArtifact must deep-copy helmValues")
	}
	spec.InstallOptions.Annotations["w7.cc/deny-delete"] = "false"
	if artifact.InstallOptions.Annotations["w7.cc/deny-delete"] != "true" {
		t.Fatal("effectiveArtifact must deep-copy annotations")
	}
}
