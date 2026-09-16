package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegistryWriteAuth protects mutable Docker Registry V2 operations while
// keeping image pulls available to nodes that do not carry a panel credential.
type RegistryWriteAuth struct{}

func (RegistryWriteAuth) Process(ctx *gin.Context) {
	if !IsRegistryWriteRequest(ctx.Request) {
		ctx.Next()
		return
	}
	Auth{}.Process(ctx)
}

func IsRegistryWriteRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	return IsRegistryWrite(req.Method, req.URL.Path)
}

func IsRegistryWrite(method, path string) bool {
	if !strings.HasPrefix(path, "/v2/") {
		return false
	}
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requiresRoutePermission(method, path string) bool {
	return strings.HasPrefix(path, "/panel-api/v1/") || IsRegistryWrite(method, path)
}
