package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

// 代理不走认证的路由 比如 proxy-no 需废弃
// proxy-no路由 占用了header Authorization
type ProxyNoAuth struct {
	middleware.Abstract
}

func (self ProxyNoAuth) Process(gin *gin.Context) {
	gin.Next()
}
