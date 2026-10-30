package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMainCostProxyMutationCoversDirectWrites(t *testing.T) {
	base := "/k8s-proxy/apis/ckm.w7.cc/v1alpha1/namespaces/default/costs"
	for _, tc := range []struct {
		method, path string
		want         bool
	}{
		{http.MethodPost, base, true},
		{http.MethodPut, base + "/panel-cost", true},
		{http.MethodPatch, base + "/panel-cost", true},
		{http.MethodDelete, base + "/panel-cost", true},
		{http.MethodPatch, base + "//panel-cost", true},
		{http.MethodGet, base + "/panel-cost", false},
		{http.MethodPatch, "/k8s-proxy/api/v1/namespaces/default/configmaps/example", false},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := mainCostProxyMutation(req); got != tc.want {
			t.Errorf("%s %s: mutation=%v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}
