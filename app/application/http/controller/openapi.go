package controller

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type OpenAPI struct {
	controller.Abstract
}

func (o OpenAPI) Page(ctx *gin.Context) {
	specURL := ctx.DefaultQuery("url", "/docs/openapi/spec")

	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.String(http.StatusOK, `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>OpenAPI Docs</title>
  <style>
    body { margin: 0; padding: 0; }
  </style>
</head>
<body>
  <redoc spec-url="`+specURL+`"></redoc>
  <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`)
}

func (o OpenAPI) Spec(ctx *gin.Context) {
	for _, filePath := range candidateOpenAPIFiles() {
		if fileExists(filePath) {
			ctx.File(filePath)
			return
		}
	}

	sdk := k8s.NewK8sClient()
	data, err := sdk.ClientSet.RESTClient().Get().AbsPath("/openapi/v2").SetHeader("Accept", "application/json").DoRaw(ctx)
	if err != nil {
		o.JsonResponse(ctx, gin.H{
			"message": "openapi spec not found",
			"error":   err.Error(),
		}, nil, http.StatusNotFound)
		return
	}

	ctx.Data(http.StatusOK, "application/json; charset=utf-8", data)
}

func candidateOpenAPIFiles() []string {
	staticPath := facade.Config.GetString("app.static_path")
	return []string{
		filepath.Join(staticPath, "assets", "openapi.json"),
		filepath.Join(staticPath, "assets", "swagger.json"),
		filepath.Join(staticPath, "openapi.json"),
		filepath.Join(staticPath, "swagger.json"),
		filepath.Join(staticPath, "schema", "openapi.json"),
		filepath.Join(staticPath, "schema", "swagger.json"),
		filepath.Join(staticPath, "schema", "openapi.yaml"),
		filepath.Join(staticPath, "schema", "swagger.yaml"),
		filepath.Join(staticPath, "schema", "openapi.yml"),
		filepath.Join(staticPath, "schema", "swagger.yml"),
	}
}

func fileExists(filePath string) bool {
	info, err := os.Stat(filePath)
	return err == nil && !info.IsDir()
}

func (o OpenAPI) RedirectJSON(ctx *gin.Context) {
	ctx.Redirect(http.StatusTemporaryRedirect, "/docs/openapi/spec")
}
