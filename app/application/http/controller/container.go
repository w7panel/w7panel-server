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
	pushReq, err := localRegistryPushRequest(params.RegistryDomain, http.Request.Host, http.MustGet("k8s_token").(string))
	if err != nil {
		self.JsonResponseWithError(http, err, nethttp.StatusBadRequest)
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
	pushReq.ImageName = params.ImageName
	pushReq.Progress = func(progress container.Progress) error {
		_, writeErr := fmt.Fprintf(http.Writer, "%s\n", progress.Content)
		if writeErr != nil {
			slog.Error("exportAndPushContainerImageByRootfs streaming error", "params", params, "err", writeErr)
			return writeErr
		}
		flusher.Flush()

		return nil
	}
	err = container.ExportAndPushContainerImageByRootfs(containerClient, params.ContainerId, pushReq)
	if err != nil {
		slog.Error("exportAndPushContainerImageByRootfs error", "params", params, "err", err)
		self.JsonResponseWithServerError(http, err)
		return
	}

	http.Writer.WriteHeader(nethttp.StatusOK)
}

func localRegistryPushRequest(registryDomain, requestHost, token string) (container.PushRequest, error) {
	if registryDomain != requestHost || token == "" {
		return container.PushRequest{}, errors.New("invalid local registry destination or credential")
	}
	return container.PushRequest{
		RegisterDomain:    "127.0.0.1:8000",
		RegistryUsername:  "w7panel-k8s-token",
		RegistryPassword:  token,
		RegistryPlainHTTP: true,
	}, nil
}
