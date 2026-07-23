package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/audit"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type Audit struct {
	middleware.Abstract
}

func (Audit) Process(ctx *gin.Context) {
	start := time.Now()
	ctx.Next()
	if shouldSkipAudit(ctx) {
		return
	}
	if !isAuditWriteMethod(ctx.Request.Method) {
		return
	}
	if ctx.GetString("k8s_token") == "" || ctx.GetString("username") == "" {
		return
	}
	audit.RecordOperation(ctx, start)
}

func isAuditWriteMethod(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK", "LINK", "UNLINK":
		return true
	default:
		return false
	}
}

func shouldSkipAudit(ctx *gin.Context) bool {
	path := ctx.Request.URL.Path
	return strings.HasPrefix(path, "/panel-api/v1/audit/")
}
