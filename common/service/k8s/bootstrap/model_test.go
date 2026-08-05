package bootstrap

import (
	"strings"
	"testing"

	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

func validInstallation() *installationv1.BootstrapInstallation {
	return &installationv1.BootstrapInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: "w7panel-default-higress", UID: types.UID("installation-uid")},
		Spec: installationv1.BootstrapInstallationSpec{
			Artifact: installationv1.ArtifactReference{Name: "higress", Type: installationv1.ArtifactTypeZPK, Identifie: "w7panel-higress", Source: "https://zpk.w7.cc/info/higress"},
			Target:   installationv1.ArtifactTarget{ReleaseName: "w7panel-higress", Namespace: "default"},
		},
	}
}

func TestValidateInstallation(t *testing.T) {
	base := validInstallation()
	tests := []struct {
		name    string
		mutate  func(*installationv1.BootstrapInstallation)
		wantErr string
	}{
		{name: "valid"},
		{name: "http rejected", mutate: func(item *installationv1.BootstrapInstallation) {
			item.Spec.Artifact.Source = "http://zpk.w7.cc/info/higress"
		}, wantErr: "仅支持 HTTPS"},
		{name: "credentials rejected", mutate: func(item *installationv1.BootstrapInstallation) {
			item.Spec.Artifact.Source = "https://user:password@zpk.w7.cc/info/higress"
		}, wantErr: "用户名或密码"},
		{name: "unsupported type", mutate: func(item *installationv1.BootstrapInstallation) { item.Spec.Artifact.Type = "Helm" }, wantErr: "仅 ZPK"},
		{name: "invalid helm value", mutate: func(item *installationv1.BootstrapInstallation) {
			item.Spec.InstallOptions.HelmValues = map[string]string{"service.type": "value,with,commas"}
		}, wantErr: "helmValues"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := base.DeepCopy()
			if test.mutate != nil {
				test.mutate(item)
			}
			err := validateInstallation(item)
			if test.wantErr == "" && err != nil {
				t.Fatalf("validateInstallation() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("validateInstallation() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestInstallationSettingsMaxRetries(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries *int32
		want       int32
	}{
		{name: "default", want: defaultMaxRetries},
		{name: "zero", maxRetries: ptr.To[int32](0), want: 0},
		{name: "custom", maxRetries: ptr.To[int32](5), want: 5},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := validInstallation()
			item.Spec.Strategy.MaxRetries = test.maxRetries
			if got := installationSettings(item).MaxRetries; got != test.want {
				t.Fatalf("MaxRetries = %d, want %d", got, test.want)
			}
		})
	}
}

func TestOperationIDUsesInstallationUID(t *testing.T) {
	item := validInstallation()
	first := operationID(item)
	item.Spec.Artifact.Version = "2.0.0"
	if second := operationID(item); second != first {
		t.Fatal("operation ID must remain stable when the spec changes")
	}
	item.UID = types.UID("another-installation-uid")
	if second := operationID(item); second == first {
		t.Fatal("operation ID must change with the installation UID")
	}
}
