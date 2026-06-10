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
	})

	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if routes[0].Path != "/panel-api/v1/example/*" {
		t.Fatalf("expected normalized path, got %q", routes[0].Path)
	}
	if routes[0].Description == "" {
		t.Fatal("expected description to be set")
	}
	if routes[0].Title != routes[0].Description {
		t.Fatalf("expected title to match description, got title=%q description=%q", routes[0].Title, routes[0].Description)
	}
}
