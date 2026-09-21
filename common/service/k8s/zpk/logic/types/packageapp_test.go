package types_test

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
)

func disabledNewPackageApps(t *testing.T) {
	uri := "https://zpk.w7.cc/zpk/respo/info/w7_zpkv2"
	manifestPackage, err := logic.LoadPackage(uri)
	if err != nil {
		t.Error(err)
	}
	/**
	Identifie                string         `json:"identifie"`
	PvcName                  string         `json:"pvc_name"`
	DockerRegistry           DockerRegistry `json:"registry"`
	DockerRegistrySecretName string         `json:"docker_registry_secret_name"`
	Namespace                string         `json:"namespace"`
	InstallId                string         `json:"installId"` //安装的id
	EnvKv                    `json:"env_kv"`
	*/
	/*

	 */
	optionMain := types.InstallOption{
		Identifie: "longflow_ai",
		Namespace: "default",
		InstallId: "install-id",
		EnvKv: []types.EnvKv{
			{Name: "DOMAIN_URL", Value: "https://test.w7.cc"},
			{Name: "LANGFLOW_DATABASE_URL", Value: "postgresql://langflow:langflow@%HOST%:5432/langflow111"},
		},
	}
	optionPg := types.InstallOption{
		Identifie: "longflow_pgsql",
		Namespace: "default",
		InstallId: "install-id",
		EnvKv: []types.EnvKv{
			{Name: "POSTGRES_USER", Value: "username2"},
		},
	}
	options := []types.InstallOption{optionMain, optionPg}

	var app = types.NewPackage(manifestPackage, options, "releasename", "install-id", "default", "", "", "")
	apps := app.Children
	if len(apps) == 0 {
		t.Error("NewPackageApps should return not empty")
	}
	for _, app := range apps {
		t.Log(app.Manifest.Platform.Container.StartParams)
	}

}

func TestGetDockerRegistryDefaultsForLocalRegistrySecret(t *testing.T) {
	app := &types.PackageApp{InstallOption: &types.InstallOption{
		Namespace:                "default",
		DockerRegistrySecretName: "registry.local.w7.cc",
	}}
	registry := app.GetDockerRegisty()
	if registry.Host != "registry.local.w7.cc" || registry.Username != "" || registry.Password != "" || registry.Namespace != "default" {
		t.Fatalf("unexpected local registry default: %#v", registry)
	}
}

func TestPackageAppDefaultDomainAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		ingressHost string
		forceHTTPS  bool
		startParams []types.StartParams
		wantDomain  string
	}{
		{
			name:        "ingress host remains the first choice",
			ingressHost: "app.example.test",
			startParams: []types.StartParams{{Name: "DOMAIN_URL", ValuesText: "dependency.example.test", ModuleName: "php-env"}},
			wantDomain:  "http://app.example.test",
		},
		{
			name:        "module domain parameter adds annotation",
			startParams: []types.StartParams{{Name: "DOMAIN_URL", ValuesText: "dependency.example.test", ModuleName: "php-env"}},
			wantDomain:  "http://dependency.example.test",
		},
		{
			name:        "explicit domain scheme is preserved",
			startParams: []types.StartParams{{Name: "domain_url", ValuesText: "https://dependency.example.test/", ModuleName: "php-env"}},
			wantDomain:  "https://dependency.example.test",
		},
		{
			name:        "forced https applies to host-only parameter",
			forceHTTPS:  true,
			startParams: []types.StartParams{{Name: "DOMAIN_URL", ValuesText: "dependency.example.test"}},
			wantDomain:  "https://dependency.example.test",
		},
		{
			name:        "ssl domain parameter uses https",
			startParams: []types.StartParams{{Name: "DOMAIN_SSL_URL", ValuesText: "dependency.example.test", ModuleName: "php-env"}},
			wantDomain:  "https://dependency.example.test",
		},
		{
			name:        "unresolved placeholder is ignored",
			startParams: []types.StartParams{{Name: "DOMAIN_URL", ValuesText: "%DOMAIN_URL%", ModuleName: "php-env"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &types.PackageApp{
				ManifestPackage: &types.ManifestPackage{Manifest: types.Manifest{
					Application: types.Application{Identifie: "plugin", Type: "app-plugin"},
					Platform:    types.Platform{Container: types.Container{StartParams: tt.startParams}},
				}},
				InstallOption: &types.InstallOption{
					ReleaseName:       "plugin-release",
					Namespace:         "default",
					IngressHost:       tt.ingressHost,
					IngressForceHttps: tt.forceHTTPS,
				},
			}

			if got := app.GetAnnotations()["w7.cc/default-domain"]; got != tt.wantDomain {
				t.Fatalf("default domain annotation = %q, want %q", got, tt.wantDomain)
			}
		})
	}
}
