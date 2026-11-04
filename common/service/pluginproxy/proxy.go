package pluginproxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler keeps legacy read URLs while the query implementation lives in its plugin.
func Handler(envName, fallback, legacyPrefix, pluginPrefix string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.Request.Method != http.MethodGet {
			ctx.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		base := os.Getenv(envName)
		if base == "" {
			base = fallback
		}
		target, err := url.Parse(base)
		if err != nil || target.Scheme != "http" && target.Scheme != "https" || target.Host == "" {
			ctx.AbortWithStatus(http.StatusBadGateway)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "plugin unavailable", http.StatusBadGateway)
		}
		path := ctx.Request.URL.Path
		original := proxy.Director
		proxy.Director = func(r *http.Request) {
			original(r)
			r.URL.Path = pluginPrefix + strings.TrimPrefix(path, legacyPrefix)
			r.URL.RawPath = ""
			r.Host = target.Host
		}
		proxy.ServeHTTP(ctx.Writer, ctx.Request)
	}
}
