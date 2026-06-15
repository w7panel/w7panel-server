package permission

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRoutesFromGinIncludesDescription(t *testing.T) {
	routes := RoutesFromGin(gin.RoutesInfo{
		{
			Method: "GET",
			Path:   "/panel-api/v1/example/:name",
		},
		{
			Method: "GET",
			Path:   "/panel-api/v1/gpu/config",
		},
		{
			Method: "POST",
			Path:   "/panel-api/v1/files/webdav-test/*path",
		},
	})

	expected := map[string]string{
		"GET /panel-api/v1/example/*":            "获取example",
		"GET /panel-api/v1/gpu/config":           "获取 GPU 配置",
		"POST /panel-api/v1/files/webdav-test/*": "测试 WebDAV 文件访问",
	}
	if len(routes) != len(expected) {
		t.Fatalf("expected %d routes, got %d", len(expected), len(routes))
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if route.Description != expected[key] {
			t.Fatalf("expected %s description %q, got %q", key, expected[key], route.Description)
		}
		if route.Title != route.Description {
			t.Fatalf("expected title to match description, got title=%q description=%q", route.Title, route.Description)
		}
	}
}
