package controller

import (
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s/container"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
)

type Container struct {
	controller.Abstract
}

func (self Container) ExportAndPushImage(http *gin.Context) {
	type ParamsValidate struct {
		ContainerId    string `form:"containerId" binding:"required"`
		RegistryDomain string `form:"registryDomain" binding:"required"`
		ImageName      string `form:"imageName" binding:"required"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	flusher, ok := http.Writer.(nethttp.Flusher)
	if !ok {
		self.JsonResponseWithServerError(http, errors.New("streaming not supported"))
		return
	}

	containerClient, err := container.GetDefaultContainerClient()
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}

	http.Writer.Header().Set("Content-Type", "text/event-stream")
	http.Writer.Header().Set("Cache-Control", "no-cache")
	http.Writer.Header().Set("Connection", "keep-alive")
	err = container.ExportAndPushContainerImageByRootfs(containerClient, params.ContainerId, container.PushRequest{
		RegisterDomain: params.RegistryDomain,
		Progress: func(progress container.Progress) error {
			_, err = fmt.Fprintf(http.Writer, "%s\n", progress.Content)
			if err != nil {
				slog.Error("exportAndPushContainerImageByRootfs streaming error", "params", params, "err", err)
				return err
			}
			flusher.Flush()

			return nil
		},
	})
	if err != nil {
		slog.Error("exportAndPushContainerImageByRootfs error", "params", params, "err", err)
		self.JsonResponseWithServerError(http, err)
		return
	}

	http.Writer.WriteHeader(nethttp.StatusOK)
}
