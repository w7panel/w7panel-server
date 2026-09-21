package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHostCheckPort9090OnlyAllowsIP(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		wantStatus int
	}{
		{name: "ipv4", host: "192.0.2.1:9090", wantStatus: 204},
		{name: "ipv6", host: "[2001:db8::1]:9090", wantStatus: 204},
		{name: "domain", host: "panel.example.com:9090", wantStatus: 400},
		{name: "localhost", host: "localhost:9090", wantStatus: 400},
		{name: "other port domain", host: "panel.example.com:8080", wantStatus: 204},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(HostCheck{}.Process)
			r.GET("/", func(c *gin.Context) { c.Status(204) })

			req := httptest.NewRequest("GET", "http://"+tt.host+"/", nil)
			res := httptest.NewRecorder()
			r.ServeHTTP(res, req)

			assert.Equal(t, tt.wantStatus, res.Code)
		})
	}
}
