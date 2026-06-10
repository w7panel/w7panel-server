package audit

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
)

func clientIP(ctx *gin.Context) string {
	if ctx == nil {
		return ""
	}
	return ctx.ClientIP()
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 512 {
		msg = msg[:512]
	}
	return msg
}

func sanitizeParams(params gin.Params) string {
	if len(params) == 0 {
		return ""
	}
	values := make(map[string]string, len(params))
	for _, param := range params {
		key := strings.ToLower(param.Key)
		if strings.Contains(key, "token") || strings.Contains(key, "password") || strings.Contains(key, "secret") {
			values[param.Key] = "***"
			continue
		}
		values[param.Key] = param.Value
	}
	data, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildOperationMessage(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return "操作"
	}
	method := ctx.Request.Method
	route := ctx.FullPath()
	if route == "" {
		route = ctx.Request.URL.Path
	}
	if description := LookupRouteDescription(method, route); description != "" {
		return description
	}
	return fallbackOperationMessage(method, route)
}

func fallbackOperationMessage(method string, route string) string {
	action := MethodDescription(method)
	if strings.TrimSpace(route) == "" {
		return action
	}
	return action + " " + route
}
