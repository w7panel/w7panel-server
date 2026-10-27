package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	mcp1 "github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/rest"
)

func TestWriteToolRegistration(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			t.Setenv("MCP_K8S_WRITE_ENABLED", enabled)
			server := httptest.NewServer(McpHttpHandler())
			defer server.Close()
			client := mcp1.NewClient(&mcp1.Implementation{Name: "test", Version: "1"}, nil)
			session, err := client.Connect(context.Background(), &mcp1.StreamableClientTransport{Endpoint: server.URL}, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			result, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			foundRead, foundWrite := false, false
			for _, tool := range result.Tools {
				foundRead = foundRead || tool.Name == "k8s_proxy_request"
				foundWrite = foundWrite || tool.Name == "k8s_proxy_write"
			}
			if !foundRead || foundWrite != (enabled == "true") {
				t.Fatalf("tools: read=%v write=%v, enabled=%s", foundRead, foundWrite, enabled)
			}
		})
	}
}

func TestValidateK8sWritePath(t *testing.T) {
	tests := []struct {
		method string
		path   string
		valid  bool
	}{
		{http.MethodPost, "/api/v1/namespaces/demo/configmaps", true},
		{http.MethodPut, "/api/v1/namespaces/demo/configmaps/app", true},
		{http.MethodPatch, "/apis/apps/v1/namespaces/demo/deployments/app/status", true},
		{http.MethodPost, "/api/v1/namespaces/demo/pods/app/eviction", true},
		{http.MethodPut, "/api/v1/namespaces/demo/finalize", true},
		{http.MethodDelete, "/api/v1/namespaces/demo/configmaps/app?dryRun=All", true},
		{http.MethodPost, "/api/v1/namespaces/demo/secrets", false},
		{http.MethodPost, "/api/v1/namespaces/demo/pods/app/exec?command=id", true},
		{http.MethodPost, "/api/v1/namespaces/demo/pods/app/attach", true},
		{http.MethodPost, "/api/v1/namespaces/demo/services/app/proxy/health", true},
		{http.MethodDelete, "/api/v1/namespaces/demo/pods/app/exec", false},
		{http.MethodPost, "/api/v1/namespaces/demo/pods/app/portforward", false},
		{http.MethodPost, "/api/v1/namespaces/demo/configmaps/app", false},
		{http.MethodPatch, "/api/v1/namespaces/demo/configmaps", false},
		{http.MethodDelete, "/api/v1/namespaces/demo/configmaps/app/status/extra", false},
		{http.MethodPost, "/api/v1/namespaces/demo/configmaps%2Fapp", false},
		{http.MethodPost, "https://elsewhere/api/v1/pods", false},
		{http.MethodGet, "/api/v1/pods", false},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			_, _, _, err := validateK8sWritePath(test.path, test.method)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v, error=%v", test.valid, err)
			}
		})
	}
}

func TestValidateK8sWriteBody(t *testing.T) {
	tests := []struct {
		params K8sWriteParams
		action string
		valid  bool
	}{
		{params: K8sWriteParams{Method: http.MethodPost, Body: `{}`}, valid: true},
		{params: K8sWriteParams{Method: http.MethodDelete}, valid: true},
		{params: K8sWriteParams{Method: http.MethodPatch, Body: `{"metadata":{"labels":{"a":"b"}}}`}, valid: true},
		{params: K8sWriteParams{Method: http.MethodPatch, ContentType: "application/json-patch+json", Body: `[{"op":"remove","path":"/metadata/labels/a"}]`}, valid: true},
		{params: K8sWriteParams{Method: http.MethodPost}, action: "exec", valid: true},
		{params: K8sWriteParams{Method: http.MethodPost, Body: "input"}, action: "attach", valid: true},
		{params: K8sWriteParams{Method: http.MethodPost, ContentType: "text/plain", Body: "raw"}, action: "proxy", valid: true},
		{params: K8sWriteParams{Method: http.MethodPost}, valid: false},
		{params: K8sWriteParams{Method: http.MethodPut, Body: `[]`}, valid: false},
		{params: K8sWriteParams{Method: http.MethodPatch, ContentType: "application/json-patch+json", Body: `{}`}, valid: false},
		{params: K8sWriteParams{Method: http.MethodPatch, ContentType: "text/plain", Body: `{}`}, valid: false},
		{params: K8sWriteParams{Method: http.MethodDelete, Body: `{bad}`}, valid: false},
		{params: K8sWriteParams{Method: http.MethodPost, Body: strings.Repeat(" ", maxK8sWriteBody+1)}, valid: false},
	}
	for _, test := range tests {
		_, err := validateK8sWriteBody(&test.params, test.action)
		if (err == nil) != test.valid {
			t.Fatalf("params=%+v valid=%v error=%v", test.params, test.valid, err)
		}
	}
}

func TestStreamLimits(t *testing.T) {
	_, err := streamK8s(context.Background(), &rest.Config{}, http.MethodPost, &url.URL{Path: "/api/v1/namespaces/demo/pods/app/exec"}, "", "exec")
	if err == nil || !strings.Contains(err.Error(), "command") {
		t.Fatalf("missing exec command: %v", err)
	}
	out := &cappedOutput{}
	if _, err := out.Write(make([]byte, maxK8sProxyResponse)); err != nil {
		t.Fatal(err)
	}
	if _, err := out.Write([]byte("x")); err == nil {
		t.Fatal("oversized stream output was accepted")
	}
}
