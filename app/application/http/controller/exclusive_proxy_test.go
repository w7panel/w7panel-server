package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExclusiveConfigMapCollectionPost(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		blocked    bool
	}{
		{"ordinary", `{"metadata":{"name":"ordinary"}}`, false},
		{"protected", `{"metadata":{"name":"w7panel-permission-exclusives"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/k8s-proxy/api/v1/namespaces/default/configmaps", strings.NewReader(tc.body))
			err := checkExclusiveProxyWrite(ctx, "/api/v1/namespaces/default/configmaps")
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked = %v, want %v: %v", err != nil, tc.blocked, err)
			}
			body, err := io.ReadAll(ctx.Request.Body)
			if err != nil || string(body) != tc.body {
				t.Fatalf("body after check = %q, %v", body, err)
			}
		})
	}
}
