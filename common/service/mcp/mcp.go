package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	mcp1 "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/k8s"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type K8sParams struct {
	Path string `json:"path" jsonschema:"Read-only Kubernetes API path beginning with /api or /apis"`
}

type K8sWriteParams struct {
	Path        string `json:"path" jsonschema:"Kubernetes resource path beginning with /api or /apis"`
	Method      string `json:"method" jsonschema:"POST, PUT, PATCH, or DELETE"`
	Body        string `json:"body,omitempty" jsonschema:"JSON request body; required except for DELETE"`
	ContentType string `json:"contentType,omitempty" jsonschema:"Optional PATCH content type: application/merge-patch+json or application/json-patch+json"`
}

type k8sProxyResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

const maxK8sProxyResponse = 256 * 1024
const maxK8sWriteBody = 1024 * 1024

func McpHttpHandler() http.Handler {
	server := mcp1.NewServer(&mcp1.Implementation{
		Name:    "panel-server",
		Version: "1.0.0",
	}, nil)

	// Add MCP-level logging middleware.
	// server.AddReceivingMiddleware(createLoggingMiddleware())

	// Add the cityTime tool.
	mcp1.AddTool(server, &mcp1.Tool{
		Name:        "k8s_proxy_request",
		Description: "使用当前用户的 Kubernetes 凭据执行只读 GET 请求；不允许访问 Secret。",
	}, k8sProxy)
	if os.Getenv("MCP_K8S_WRITE_ENABLED") == "true" {
		mcp1.AddTool(server, &mcp1.Tool{
			Name:        "k8s_proxy_write",
			Description: "使用当前用户的 Kubernetes 凭据创建、更新、修改或删除资源及子资源（含 exec、attach、proxy）；不允许访问 Secret。",
		}, k8sProxyWrite)
	}

	// Create the streamable HTTP handler.
	handler := mcp1.NewStreamableHTTPHandler(func(req *http.Request) *mcp1.Server {
		return server
	}, nil)
	return handler
}

func k8sProxy(ctx context.Context, req *mcp1.CallToolRequest, params *K8sParams) (*mcp1.CallToolResult, any, error) {
	if params == nil {
		return nil, nil, fmt.Errorf("request parameters are required")
	}
	path, query, err := validateK8sProxyPath(params.Path)
	if err != nil {
		return nil, nil, err
	}
	return proxyK8s(ctx, req, http.MethodGet, path, query, "", "", "")
}

func k8sProxyWrite(ctx context.Context, req *mcp1.CallToolRequest, params *K8sWriteParams) (*mcp1.CallToolResult, any, error) {
	if params == nil {
		return nil, nil, fmt.Errorf("request parameters are required")
	}
	path, query, action, err := validateK8sWritePath(params.Path, params.Method)
	if err != nil {
		return nil, nil, err
	}
	contentType, err := validateK8sWriteBody(params, action)
	if err != nil {
		return nil, nil, err
	}
	return proxyK8s(ctx, req, params.Method, path, query, params.Body, contentType, action)
}

func proxyK8s(ctx context.Context, req *mcp1.CallToolRequest, method, path, query, body, contentType, action string) (*mcp1.CallToolResult, any, error) {
	var err error
	token, ok := middleware.K8sTokenFromContext(ctx)
	if !ok {
		token, err = bearerToken(req)
	}
	if err != nil {
		return nil, nil, err
	}

	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		return nil, nil, err
	}
	config, err := sdk.ToRESTConfig()
	if err != nil {
		return nil, nil, err
	}
	base, err := url.Parse(config.Host)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid Kubernetes API server URL: %w", err)
	}
	target := base.ResolveReference(&url.URL{Path: path, RawQuery: query})
	if action == "exec" || action == "attach" {
		result, err := streamK8s(ctx, config, method, target, body, action)
		if err != nil {
			return nil, nil, err
		}
		return k8sToolResult(result)
	}
	transport, err := rest.TransportFor(config)
	if err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), strings.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxK8sProxyResponse+1))
	if err != nil {
		return nil, nil, err
	}
	if len(responseBody) > maxK8sProxyResponse {
		return nil, nil, fmt.Errorf("Kubernetes response exceeds %d bytes", maxK8sProxyResponse)
	}

	return k8sToolResult(k8sProxyResult{Status: response.StatusCode, Body: string(responseBody)})
}

func k8sToolResult(result k8sProxyResult) (*mcp1.CallToolResult, any, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &mcp1.CallToolResult{Content: []mcp1.Content{&mcp1.TextContent{Text: string(encoded)}}}, result, nil
}

type cappedOutput struct {
	mu sync.Mutex
	bytes.Buffer
}

func (out *cappedOutput) Write(p []byte) (int, error) {
	out.mu.Lock()
	defer out.mu.Unlock()
	if out.Len()+len(p) > maxK8sProxyResponse {
		return 0, fmt.Errorf("Kubernetes response exceeds %d bytes", maxK8sProxyResponse)
	}
	return out.Buffer.Write(p)
}

func (out *cappedOutput) String() string {
	out.mu.Lock()
	defer out.mu.Unlock()
	return out.Buffer.String()
}

func streamK8s(ctx context.Context, config *rest.Config, method string, target *url.URL, body, action string) (k8sProxyResult, error) {
	query := target.Query()
	if action == "exec" && len(query["command"]) == 0 {
		return k8sProxyResult{}, fmt.Errorf("exec requires at least one command parameter")
	}
	tty := query.Get("tty") == "true"
	query.Set("stdin", fmt.Sprintf("%t", body != ""))
	query.Set("stdout", "true")
	query.Set("stderr", fmt.Sprintf("%t", !tty))
	target.RawQuery = query.Encode()
	executor, err := remotecommand.NewSPDYExecutor(config, method, target)
	if err != nil {
		return k8sProxyResult{}, err
	}
	streamCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out := &cappedOutput{}
	options := remotecommand.StreamOptions{Stdout: out, Tty: tty}
	if !tty {
		options.Stderr = out
	}
	if body != "" {
		options.Stdin = strings.NewReader(body)
	}
	err = executor.StreamWithContext(streamCtx, options)
	if err != nil {
		status := http.StatusInternalServerError
		var apiStatus apierrors.APIStatus
		if errors.As(err, &apiStatus) && apiStatus.Status().Code != 0 {
			status = int(apiStatus.Status().Code)
		} else if errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		return k8sProxyResult{Status: status, Body: out.String() + err.Error()}, nil
	}
	return k8sProxyResult{Status: http.StatusOK, Body: out.String()}, nil
}

func validateK8sWritePath(rawPath, method string) (string, string, string, error) {
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return "", "", "", fmt.Errorf("method must be POST, PUT, PATCH, or DELETE")
	}
	path, query, err := validateK8sProxyPath(rawPath)
	if err != nil {
		return "", "", "", err
	}
	parsed, _ := url.Parse(rawPath)
	if parsed.RawPath != "" {
		return "", "", "", fmt.Errorf("encoded Kubernetes resource paths are not allowed")
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	prefix := 2
	if segments[0] == "apis" {
		prefix = 3
	}
	if len(segments) <= prefix {
		return "", "", "", fmt.Errorf("path must identify a Kubernetes resource")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", "", "", fmt.Errorf("invalid Kubernetes resource path")
		}
	}
	resource := segments[prefix:]
	if resource[0] == "namespaces" && len(resource) >= 3 {
		if len(resource) != 3 || resource[2] != "status" && resource[2] != "finalize" && resource[2] != "proxy" {
			resource = resource[2:]
		}
	}
	action := ""
	if len(resource) >= 3 {
		action = resource[2]
	}
	if len(resource) > 3 && action != "proxy" || len(resource) > 2 && action == "portforward" {
		return "", "", "", fmt.Errorf("invalid Kubernetes subresource path")
	}
	if action == "exec" || action == "attach" {
		if len(resource) != 3 || method != http.MethodPost {
			return "", "", "", fmt.Errorf("exec and attach require POST on a subresource")
		}
	} else if method == http.MethodPost && len(resource) != 1 && len(resource) < 3 || method != http.MethodPost && len(resource) < 2 {
		return "", "", "", fmt.Errorf("POST requires a collection or subresource; PUT, PATCH, and DELETE require an object or subresource")
	}
	if action != "exec" && action != "attach" && action != "proxy" {
		action = ""
	}
	return path, query, action, nil
}

func validateK8sWriteBody(params *K8sWriteParams, action string) (string, error) {
	if len(params.Body) > maxK8sWriteBody {
		return "", fmt.Errorf("Kubernetes request body exceeds %d bytes", maxK8sWriteBody)
	}
	if action == "exec" || action == "attach" {
		if params.ContentType != "" {
			return "", fmt.Errorf("streaming subresources do not accept a content type")
		}
		return "", nil
	}
	if action == "proxy" {
		return params.ContentType, nil
	}
	contentType := "application/json"
	if params.Method == http.MethodPatch {
		contentType = "application/merge-patch+json"
	}
	if params.ContentType != "" {
		contentType = params.ContentType
	}
	if params.Method == http.MethodPatch {
		if contentType != "application/merge-patch+json" && contentType != "application/json-patch+json" {
			return "", fmt.Errorf("unsupported PATCH content type")
		}
	} else if contentType != "application/json" {
		return "", fmt.Errorf("unsupported Kubernetes content type")
	}
	if params.Body == "" && params.Method == http.MethodDelete {
		return "", nil
	}
	if !json.Valid([]byte(params.Body)) {
		return "", fmt.Errorf("body must be valid JSON")
	}
	var value any
	if err := json.Unmarshal([]byte(params.Body), &value); err != nil {
		return "", err
	}
	if contentType == "application/json-patch+json" {
		if _, ok := value.([]any); !ok {
			return "", fmt.Errorf("JSON Patch body must be an array")
		}
	} else if _, ok := value.(map[string]any); !ok {
		return "", fmt.Errorf("body must be a JSON object")
	}
	return contentType, nil
}

func validateK8sProxyPath(rawPath string) (string, string, error) {
	if rawPath == "" {
		return "", "", fmt.Errorf("path is required")
	}
	parsed, err := url.Parse(rawPath)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("invalid Kubernetes API path")
	}
	path := parsed.Path
	if path != "/api" && !strings.HasPrefix(path, "/api/") && path != "/apis" && !strings.HasPrefix(path, "/apis/") {
		return "", "", fmt.Errorf("only /api and /apis paths are allowed")
	}
	if strings.Contains(strings.ToLower(path), "secret") || strings.Contains(path, "..") {
		return "", "", fmt.Errorf("Secret resources and path traversal are not allowed")
	}
	return path, parsed.RawQuery, nil
}

func bearerToken(req *mcp1.CallToolRequest) (string, error) {
	if req == nil || req.Extra == nil {
		return "", fmt.Errorf("Kubernetes authorization is required")
	}
	authorization := req.Extra.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, "Bearer ") {
		return "", fmt.Errorf("Kubernetes authorization must be a Bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token == "" {
		return "", fmt.Errorf("Kubernetes authorization must be a Bearer token")
	}
	return token, nil

}
