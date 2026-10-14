package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type Cors struct {
	middleware.Abstract
}

func (self Cors) Clear(ctx *gin.Context) {
	ctx.Header("Access-Control-Allow-Origin", "")
	ctx.Header("Access-Control-Allow-Headers", "")
	ctx.Header("Access-Control-Allow-Methods", "")
	ctx.Header("Access-Control-Expose-Headers", "")
	ctx.Header("Access-Control-Allow-Credentials", "")
}

func (self Cors) CorsHandle(ctx *gin.Context) {
	if host, ok := self.isAllow(ctx); ok {
		self.Clear(ctx)
		ctx.Header("Access-Control-Allow-Origin", host)
		ctx.Header("Access-Control-Allow-Headers", "Content-Type,AccessToken,X-CSRF-Token, Authorization, X-W7Panel-K8s-Token, Accept, depth, Cache-Control, X-Requested-With")
		ctx.Header("Access-Control-Allow-Methods", "POST, GET, PUT, PATCH, DELETE, HEAD, OPTIONS, PROPFIND, PROPPATCH, MKCOL, COPY, MOVE, LOCK, UNLOCK, LINK, UNLINK")
		ctx.Header("Access-Control-Expose-Headers", self.getAllowHeader())
		ctx.Header("Access-Control-Allow-Credentials", "true")
	}
}

func (self Cors) Process(ctx *gin.Context) {
	// 对于OPTIONS请求，立即处理并返回
	self.CorsHandle(ctx)
	if ctx.Request.Method == "OPTIONS" {
		ctx.AbortWithStatus(http.StatusNoContent)
		return
	}
	// 对于其他请求，先执行后续中间件
	ctx.Next()

}

func (self Cors) isAllow(ctx *gin.Context) (string, bool) {
	host := ctx.Request.Header.Get("origin")
	if host == "" {
		host = ctx.Request.Header.Get("referer")
	}
	if host == "" {
		return "", false
	}
	return host, true
}

func (self Cors) getAllowHeader() string {
	allowHeader := []string{
		"Content-Length",
		"Content-Type",
		"X-Auth-Token",
		"Origin",
		"Authorization",
		"X-W7Panel-K8s-Token",
		"X-Requested-With",
		"x-requested-with",
		"x-xsrf-token",
		"x-csrf-token",
		"x-w7-from",
		"access-token",
		"Api-Version",
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Headers",
		"Access-Control-Allow-Methods",
		"authority",
		"uid",
		"uuid",
		"Accept",
		"Cache-Control",
		"depth",
	}
	return strings.Join(allowHeader, ",")
}
