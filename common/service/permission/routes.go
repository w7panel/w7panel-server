package permission

import (
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	auditservice "github.com/w7panel/w7panel/common/service/audit"
)

type APIRoute struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Verb        string `json:"verb"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

func RoutesFromGin(routes gin.RoutesInfo) []APIRoute {
	seen := map[string]bool{}
	result := []APIRoute{}
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/panel-api/v1/") {
			continue
		}
		if strings.HasPrefix(route.Path, "/panel-api/v1/noauth/") {
			continue
		}
		verb := verbForHTTPMethod(route.Method)
		if verb == "" {
			continue
		}
		path := normalizeGinRoutePath(route.Path)
		description := routeDescription(route.Method, route.Path, path)
		item := APIRoute{
			Method:      route.Method,
			Path:        path,
			Verb:        verb,
			Title:       description,
			Description: description,
		}
		key := item.Method + " " + item.Path
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path == result[j].Path {
			return result[i].Method < result[j].Method
		}
		return result[i].Path < result[j].Path
	})
	return result
}

func routeDescription(method, rawPath string, normalizedPath string) string {
	if title := auditservice.LookupRouteDescription(method, rawPath); title != "" {
		return title
	}
	if title := auditservice.LookupRouteDescription(method, normalizedPath); title != "" {
		return title
	}
	return auditservice.MethodDescription(method)
}

func normalizeGinRoutePath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "*"
		}
	}
	return strings.Join(parts, "/")
}

func verbForHTTPMethod(method string) string {
	switch strings.ToUpper(method) {
	case "GET", "HEAD":
		return "get"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "patch"
	case "DELETE":
		return "delete"
	default:
		return ""
	}
}
