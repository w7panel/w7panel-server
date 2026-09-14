package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type K8sFilter struct {
	middleware.Abstract
}

func (self K8sFilter) Process(ctx *gin.Context) {
	ctx.Next()
}
