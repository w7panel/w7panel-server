package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestChildWhitelistPermissionCreate(t *testing.T) {
	for _, tc := range []struct {
		name, body     string
		child, blocked bool
	}{
		{"child without whitelist", `{"spec":{}}`, true, false},
		{"child with whitelist", `{"spec":{"domainWhiteList":[{"domain":"example.com"}]}}`, true, true},
		{"main with whitelist", `{"spec":{"domainWhiteList":[{"domain":"example.com"}]}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IS_CHILD", map[bool]string{true: "true", false: "false"}[tc.child])
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/k8s-proxy/apis/w7panel.w7.com/v1alpha1/permissions", strings.NewReader(tc.body))
			err := checkChildWhitelistProxyWrite(ctx, "/apis/w7panel.w7.com/v1alpha1/permissions")
			if (err != nil) != tc.blocked {
				t.Fatalf("blocked = %v, want %v: %v", err != nil, tc.blocked, err)
			}
			if tc.child {
				body, err := io.ReadAll(ctx.Request.Body)
				if err != nil || string(body) != tc.body {
					t.Fatalf("body after check = %q, %v", body, err)
				}
			}
		})
	}
}
