package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExclusiveMenusPutRequiresFounder(t *testing.T) {
	for _, role := range []string{"normal", "founder"} {
		w := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/panel-api/v1/auth/exclusive-menus", strings.NewReader(`{"rules":[{"menuRule":"bad/*","clusterTypes":["main"]}]}`))
		ctx.Set("user_mode", role)
		ExclusiveMenus{}.Put(ctx)
		want := http.StatusBadRequest
		if role == "normal" {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatalf("role %s: status=%d, want %d", role, w.Code, want)
		}
	}
}
