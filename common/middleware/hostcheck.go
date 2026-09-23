package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
)

type HostCheck struct {
	middleware.Abstract
}

func (self HostCheck) Process(c *gin.Context) {
	if host, port, err := net.SplitHostPort(c.Request.Host); err == nil && port == "9090" {
		if net.ParseIP(host) == nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
	} else if strings.HasSuffix(c.Request.Host, ":9090") {
		// SplitHostPort rejects malformed hosts. Treat them as invalid rather than
		// allowing a non-IP Host header through on port 9090.
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	c.Next()
}
