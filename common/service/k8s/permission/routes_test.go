package permission

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRoutesFromGinReturnsStructuredRoutes(t *testing.T) {
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
		"GET /panel-api/v1/example/*":            "get",
		"GET /panel-api/v1/gpu/config":           "get",
		"POST /panel-api/v1/files/webdav-test/*": "create",
	}
	if len(routes) != len(expected) {
		t.Fatalf("expected %d routes, got %d", len(expected), len(routes))
	}
	for _, route := range routes {
		key := route.Method + " " + route.Path
		if route.Verb != expected[key] {
			t.Fatalf("expected %s verb %q, got %q", key, expected[key], route.Verb)
		}
	}
}
