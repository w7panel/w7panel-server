package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegistryWriteAuth protects mutable Docker Registry V2 operations while
// keeping image pulls available to nodes that do not carry a panel credential.
type RegistryWriteAuth struct{}

const registryServiceAccountUsername = "w7panel-k8s-token"

func (RegistryWriteAuth) Process(ctx *gin.Context) {
	if !IsRegistryWriteRequest(ctx.Request) {
		ctx.Next()
		return
	}
	if strings.HasPrefix(strings.ToLower(ctx.GetHeader("Authorization")), "basic ") {
		token, ok := registryServiceAccountToken(ctx.Request)
		if !ok {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "请登录"})
			return
		}
		Auth{}.ProcessKubernetesToken(ctx, token)
		return
	}
	Auth{}.Process(ctx)
}

func registryServiceAccountToken(req *http.Request) (string, bool) {
	if req == nil {
		return "", false
	}
	username, password, ok := req.BasicAuth()
	if !ok || username != registryServiceAccountUsername || password == "" {
		return "", false
	}
	return password, true
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
