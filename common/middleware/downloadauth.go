package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/downloadticket"
)

// DownloadAuth accepts either normal header authentication or a narrowly
// scoped opaque download grant. Query JWTs are never accepted.
type DownloadAuth struct{}

func (DownloadAuth) Process(ctx *gin.Context) {
	path := strings.TrimPrefix(ctx.Request.URL.Path, "/panel-api/v1/download/")
	if ticket := ctx.Query("download-ticket"); ticket != "" {
		if downloadticket.Consume(ticket, path) {
			ctx.Next()
			return
		}
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "msg": "下载票据无效或已过期"})
		return
	}
	PanelAuth{}.Process(ctx)
}
