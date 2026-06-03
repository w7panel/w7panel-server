package audit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildOperationMessageUsesRouteDescription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/panel-api/v1/yaml", func(c *gin.Context) {
		got := buildOperationMessage(c)
		if got != "提交 YAML" {
			t.Fatalf("unexpected message: %s", got)
		}
	})
	w := performAuditRequest(router, http.MethodPost, "/panel-api/v1/yaml")
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func TestBuildOperationMessageFallsBackForUnmatchedRouteDescription(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/panel-api/v1/unlisted-action", func(c *gin.Context) {
		got := buildOperationMessage(c)
		want := "创建或提交 /panel-api/v1/unlisted-action"
		if got != want {
			t.Fatalf("unexpected message:\nwant: %s\ngot:  %s", want, got)
		}
	})
	w := performAuditRequest(router, http.MethodPost, "/panel-api/v1/unlisted-action")
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", w.Code)
	}
}

func performAuditRequest(router *gin.Engine, method string, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(method, path, nil)
	router.ServeHTTP(w, req)
	return w
}
