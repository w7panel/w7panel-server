package logic

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/w7panel/w7panel/app/zpk/logic/types"
	"github.com/w7panel/w7panel/common/service/k8s"
	"k8s.io/client-go/rest"
)

func TestExecuteInstallSharedPreparation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api":
			w.Write([]byte(`{"kind":"APIVersions","apiVersion":"v1","versions":["v1"]}`))
		case "/apis":
			w.Write([]byte(`{"kind":"APIGroupList","apiVersion":"v1","groups":[{"name":"w7panel.w7.com","versions":[{"groupVersion":"w7panel.w7.com/v1alpha1","version":"v1alpha1"}],"preferredVersion":{"groupVersion":"w7panel.w7.com/v1alpha1","version":"v1alpha1"}}]}`))
		case "/apis/w7panel.w7.com/v1alpha1":
			w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"w7panel.w7.com/v1alpha1","resources":[{"name":"appgroups","kind":"AppGroup","namespaced":true,"verbs":["get"]}]}`))
		case "/api/v1":
			w.Write([]byte(`{"kind":"APIResourceList","apiVersion":"v1","groupVersion":"v1","resources":[{"name":"namespaces","kind":"Namespace","namespaced":false,"verbs":["get","create"]}]}`))
		case "/api/v1/namespaces":
			if r.Method != "POST" {
				t.Errorf("unexpected method: %s", r.Method)
			}
			w.WriteHeader(201)
			w.Write([]byte(`{"apiVersion":"v1","kind":"Namespace","metadata":{"name":"target"}}`))
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
		}
	}))
	defer server.Close()
	sdk, err := k8s.NewForRestConfig(&rest.Config{Host: server.URL}, "target")
	if err != nil {
		t.Fatal(err)
	}
	for _, helm := range []bool{false, true} {
		t.Run(map[bool]string{false: "zpk", true: "helm"}[helm], func(t *testing.T) {
			manifest := HelmManifestApp(&HelmMemory{Identifie: "shared-app", ChartName: "app", Version: "1.0.0"})
			wantName, wantNamespace := "my-app", "target"
			if !helm {
				manifest.Application.Type = "app"
			} else {
				manifest.Application.Once = true
				manifest.Platform.Container.Env = []types.Env{{Name: "HELM_NAMESPACE", Value: "helm-ns"}}
				wantName, wantNamespace = "shared-app", "helm-ns"
			}
			NewManifestSingleton().Put("shared-test", &manifest)
			called := false
			request := InstallRequest{Namespace: "target", RepoUrl: "memory://shared-test", ReleaseName: "My_APP", IngressHost: "HTTPS://EXAMPLE.COM/",
				IsTrandition: true, ZipUrl: "https://example.com/code.zip", InstallOptions: []types.InstallOption{{Identifie: "shared-app", Replicas: 1}}}
			result, err := ExecuteInstall(request, InstallExecution{SDK: sdk, InstallID: "fixed", install: func(p types.Package, name, ns string) error {
				called = true
				if name != wantName || ns != wantNamespace || p.Root.InstallId != "fixed" || p.Root.IngressHost != "example.com" || p.Root.ZipUrl != request.ZipUrl {
					t.Fatalf("unexpected package: name=%s ns=%s", name, ns)
				}
				if p.Root.BuildImageSuccessUrl != "" || p.Root.K8sToken != nil {
					t.Fatal("CRD inherited HTTP context")
				}
				return nil
			}})
			if err != nil || !called || result.ReleaseName != wantName || result.Namespace != wantNamespace || result.InstallID != "fixed" {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestExecuteInstallRejectsMissingSDK(t *testing.T) {
	if _, err := ExecuteInstall(InstallRequest{}, InstallExecution{}); err == nil {
		t.Fatal("missing SDK accepted")
	}
}

func TestInstallRequestWireCompatibility(t *testing.T) {
	var request InstallRequest
	if err := json.Unmarshal([]byte(`{"namespace":"ns","repoUrl":"https://repo","releaseName":"app","installOptions":[],"ingressSeletorName":"selector","ingressClass":"higress","ingressForceHttps":true,"thirdpartyCDToken":"secret","isTrandition":true,"reinstall":true}`), &request); err != nil {
		t.Fatal(err)
	}
	if request.IngressSeletorName != "selector" || request.IngressClassName != "higress" || !request.IsTrandition || !request.Reinstall || request.ThirdpartyCDToken != "secret" {
		t.Fatalf("%+v", request)
	}
	data, err := json.Marshal(InstallResult{ReleaseName: "app", InstallID: "id", Namespace: "ns"})
	if err != nil || !strings.Contains(string(data), `"installId":"id"`) {
		t.Fatalf("%s %v", data, err)
	}
	var conflict *ArtifactInstallConflictError
	if !errors.As(&ArtifactInstallConflictError{}, &conflict) {
		t.Fatal("conflict error type lost")
	}
}
