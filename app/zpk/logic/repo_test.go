package logic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/w7panel/w7panel/app/zpk/logic/types"
	zpktypes "github.com/w7panel/w7panel/app/zpk/logic/types"
)

func TestZPKRequestDoesNotForwardPanelToken(t *testing.T) {
	header := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		header <- request.Header.Get("X-W7Panel-Token")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"data":{"manifest":"{}"}}`))
	}))
	defer server.Close()

	repository := NewRepo(server.URL+"/zpk/respo/info/demo", "", "")
	repository.SetPanelToken("cluster-service-account-token")
	_, _ = repository.loadPackageByHttp(context.Background(), repository.repoUrl, "", true)

	if got := <-header; got != "" {
		t.Fatalf("X-W7Panel-Token = %q, want empty", got)
	}
}

func TestLoadPackage(t *testing.T) {
	type args struct {
		uri string
	}
	tests := []struct {
		name    string
		args    args
		want    *zpktypes.ManifestPackage
		wantErr bool
	}{
		{
			name: "test",
			args: args{
				uri: "https://zpk.w7.cc/respo/info/nvidia_gpuoperator",
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadPackage(tt.args.uri)
			if (err != nil) != tt.wantErr {
				t.Log(err)
				// t.Errorf("LoadPackage() error = %v, wantErr %v", err, tt.wantErr)
			}

			optionMain := types.InstallOption{
				Identifie: "nvidia_gpuoperator",
				PvcName:   "longflow-ai",
				EnvKv: []types.EnvKv{
					{Name: "image.tag", Value: "v2.1.0"},
				},
			}
			// option2 := types.InstallOption{
			// 	Identifie: "ai_ollamaapi",
			// 	PvcName:   "longflow-ai",
			// 	EnvKv: []types.EnvKv{
			// 		{Name: "image.tag", Value: "v2.1.0"},
			// 	},
			// }
			options := []types.InstallOption{optionMain}

			var apps = types.NewPackage(got, options, "ai-ollamaui", "install-id", "default", "ollama.cc", "ollama", "traefik")
			apps.Root.GetServiceLbPort()
			// got.GetServiceLbPort()
			// if (err != nil) != tt.wantErr {
			// 	t.Errorf("LoadPackage() error = %v, wantErr %v", err, tt.wantErr)
			// 	return
			// }
			// if !reflect.DeepEqual(got, tt.want) {
			// 	t.Errorf("LoadPackage() = %v, want %v", got, tt.want)
			// }
			if got.Manifest.Application.Identifie != "longflow_ai" {
				t.Errorf("got.Manifest.Application.Identifie = %v, want %v", got.Manifest, tt.want)
			}
			if len(got.Children) == 0 {
				t.Errorf("got.Children = %v, want %v", len(got.Children), tt.want)
			}
		})
	}
}

func TestToPackageAddConfig(t *testing.T) {

	// uri := "https://zpk.w7.cc/respo/info/ai_ollamaui/ai_ollamaapi"
	uri := "https://zpk.w7.cc/zpk/respo/info/w7_minio"
	manifestPackage, err := LoadPackage(uri)
	if err != nil {
		t.Error(err)
	}
	config := manifestPackage.ToPackageAddConfig("", false)
	t.Log(config)
}

func TestGen(t *testing.T) {

	single := NewManifestSingleton()
	helmMemory := &HelmMemory{
		Identifie:   "helm-test",
		Title:       "helm-test1",
		Icon:        "helm-test2",
		Description: "helm-test3",
		ChartName:   "helm-test4",
		Repository:  "helm-test5",
		Version:     "helm-test6",
		Kv: []types.EnvKv{
			{Name: "helm-test7", Value: "helm-test8"},
		},
	}
	app := HelmManifestApp(helmMemory)
	single.Put(app.Application.Identifie, &app)

	newApp, ok := single.Get("helm-test")
	if !ok {
		t.Error("not found")
	}
	assert.Equal(t, "helm-test", newApp.Application.Identifie)
	assert.Equal(t, "helm-test1", newApp.Application.Name)

	uri := "memory://helm-test"
	u, err := url.Parse(uri)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "helm-test", u.Host)

}

func TestUrl(t *testing.T) {
	uri := "https://zpk.w7.cc/respo/info/ai_ollamaui/ai_ollamaapi"
	u, err := url.Parse(uri)
	if err != nil {
		t.Error(err)
	}
	assert.Equal(t, "ai_ollamaui", u.Path)
}

// deploy://console/7201/17995
func TestLoadRepo(t *testing.T) {
	repo := NewRepo("deploy://console/7201/17995", "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJpc3MiOiJodHRwczovL2NvbnNvbGUudzcuY2MvYXBpL3RoaXJkcGFydHktY2QvazhzLW9mZmxpbmUvb3BlbmlkLXRvLWNkLXRva2VuIiwiaWF0IjoxNzgzNjY4NjM0LCJleHAiOjE3ODM2NzQwMzQsIm5iZiI6MTc4MzY2ODYzNCwianRpIjoiN3FrMzlNTm1HWlV5N3V0SiIsInN1YiI6Ijc2MDUyIiwicHJ2IjoiZjBkMTkwZjdhZjJlODdjZTZmMDE2YWE4MjA1MGZjNzBmMjNmNTk1YyIsIm9wZW5faWQiOiJDSmd0X2hSMlZzRDdoX1Ftc0FBZ3pBIiwiZm91bmRlcl9vcGVuaWQiOiJDSmd0X2hSMlZzRDdoX1Ftc0FBZ3pBIiwicm9sZV9pZGVudGlmeSI6ImZvdW5kZXIiLCJvcmlnaW5fYXBwaWQiOiIzMTQ4OTkiLCJuaWNrbmFtZSI6InRtcCIsInVzZXJfaWQiOjc2MDUyLCJpc192YWxpZCI6dHJ1ZX0.KUdZ_J0X2_zrUili5gCYFm1toKFuCu2a9Rly91yO5Wk", "")
	manifest, err := repo.Load()
	if err != nil {
		t.Error(err)
	}
	t.Log(manifest.Manifest.Application.Identifie) /// w7_pros_28694
}
