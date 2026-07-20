package bootstrap

import (
	"strings"
	"testing"

	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateProfile(t *testing.T) {
	t.Setenv("BOOTSTRAP_ALLOWED_SOURCE_HOSTS", "packages.example.com")
	base := bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile"},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision: "1.0.0-1",
			Artifacts: []bootstrapv1.BootstrapArtifact{
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
			profile.Spec.Artifacts[1].ReleaseName = "domain"
		}, wantErr: "相同安装目标"},
		{name: "missing dependency", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[1].DependsOn = []string{"missing"}
		}, wantErr: "不存在的依赖"},
		{name: "dependency cycle", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].DependsOn = []string{"rate"}
		}, wantErr: "依赖存在环"},
		{name: "http rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].Source = "http://packages.example.com/info/domain"
		}, wantErr: "仅允许 HTTPS"},
		{name: "oci rejected until executor is available", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].Source = "oci://packages.example.com/domain"
		}, wantErr: "仅允许 HTTPS"},
		{name: "credentials in source rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].Source = "https://user:password@packages.example.com/info/domain"
		}, wantErr: "不能包含用户名或密码"},
		{name: "reserved helm type rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].Type = "Helm"
		}, wantErr: "仅 ZPK 执行器已启用"},
		{name: "invalid helm value rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].InstallOptions.HelmValues = map[string]string{"service.type": "value,with,commas"}
		}, wantErr: "helmValues"},
		{name: "removed retain policy rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].RemovalPolicy = bootstrapv1.RemovalPolicy("Retain")
		}, wantErr: "removalPolicy"},
		{name: "removed never policy rejected", mutate: func(profile *bootstrapv1.BootstrapProfile) {
			profile.Spec.Artifacts[0].ReinstallPolicy = bootstrapv1.ReinstallPolicy("Never")
		}, wantErr: "reinstallPolicy"},
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

func TestEffectiveArtifactDefaultsTypeAndCopiesHelmValues(t *testing.T) {
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile"},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1"},
	}
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "domain", Identifie: "domain", Source: "https://zpk.w7.cc/domain",
		ReleaseName: "domain", Namespace: "default",
		InstallOptions: bootstrapv1.BootstrapInstallOptions{
			HelmValues: map[string]string{"service.type": "ClusterIP"},
		},
	}

	spec := effectiveArtifact(profile, artifact)
	if spec.Artifact.Type != bootstrapv1.ArtifactTypeZPK {
		t.Fatalf("artifact type = %q, want %q", spec.Artifact.Type, bootstrapv1.ArtifactTypeZPK)
	}
	if spec.RemovalPolicy != bootstrapv1.RemovalPolicyUninstall || spec.ReinstallPolicy != bootstrapv1.ReinstallPolicyRequired {
		t.Fatalf("default policies = removal %q, reinstall %q", spec.RemovalPolicy, spec.ReinstallPolicy)
	}
	spec.InstallOptions.HelmValues["service.type"] = "LoadBalancer"
	if artifact.InstallOptions.HelmValues["service.type"] != "ClusterIP" {
		t.Fatal("effectiveArtifact must deep-copy helmValues")
	}
}

func TestDecideVersion(t *testing.T) {
	tests := []struct {
		name           string
		installed      string
		target         string
		allowDowngrade bool
		want           versionDecision
	}{
		{name: "install", target: "1.0.0", want: decisionInstall},
		{name: "same", installed: "v1.0.0", target: "1.0.0", want: decisionSkip},
		{name: "upgrade", installed: "1.0.0", target: "1.1.0", want: decisionUpgrade},
		{name: "ahead", installed: "2.0.0", target: "1.1.0", want: decisionAhead},
		{name: "downgrade explicitly allowed", installed: "2.0.0", target: "1.1.0", allowDowngrade: true, want: decisionUpgrade},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := decideVersion(test.installed, test.target, test.allowDowngrade); got != test.want {
				t.Fatalf("decideVersion() = %q, want %q", got, test.want)
			}
		})
	}
}
