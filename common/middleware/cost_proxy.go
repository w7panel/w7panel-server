package middleware

import (
	"net/http"
	"strings"
)

func mainCostProxyMutation(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	switch req.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return false
	}
	path := req.URL.Path
	return strings.HasPrefix(path, "/k8s-proxy/apis/ckm.w7.cc/v1alpha1/namespaces/") && strings.Contains(path, "/costs")
}
