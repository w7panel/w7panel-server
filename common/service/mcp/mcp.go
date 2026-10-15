package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	mcp1 "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/w7panel/w7panel/common/middleware"
	"github.com/w7panel/w7panel/common/service/k8s"
	"k8s.io/client-go/rest"
)

type K8sParams struct {
	Path string `json:"path" jsonschema:"Read-only Kubernetes API path beginning with /api or /apis"`
}

type k8sProxyResult struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

const maxK8sProxyResponse = 256 * 1024

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
	transport, err := rest.TransportFor(config)
	if err != nil {
		return nil, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.ResolveReference(&url.URL{Path: path, RawQuery: query}).String(), nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxK8sProxyResponse+1))
	if err != nil {
		return nil, nil, err
	}
	if len(body) > maxK8sProxyResponse {
		return nil, nil, fmt.Errorf("Kubernetes response exceeds %d bytes", maxK8sProxyResponse)
	}

	result := k8sProxyResult{Status: response.StatusCode, Body: string(body)}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}
	return &mcp1.CallToolResult{Content: []mcp1.Content{&mcp1.TextContent{Text: string(encoded)}}}, result, nil
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
