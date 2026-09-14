package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type Proxy struct {
	middleware.Abstract
}

func (self Proxy) Process(gin *gin.Context) {
	gin.Next()
}
