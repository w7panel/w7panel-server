package console

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s/container"
	"github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type appCommandArgs struct {
	ContainerId      string
	OutputTar        string
	Mode             string
	PushRef          string
	RegistryUsername string
	RegistryPassword string
	PlainHTTP        bool
}

var argsValue appCommandArgs

type Build struct {
	console.Abstract
}

func (pack Build) GetName() string {
	return "build:container:image"
}

func (pack Build) GetDescription() string {
	return "build container image"
}

func (c Build) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&argsValue.ContainerId, "container-id", "", "container id")
	cmd.Flags().StringVar(&argsValue.OutputTar, "output-tar", "./test.tar", "output tar path")
}

func (pack Build) Handle(cmd *cobra.Command, args []string) {
	saveFile, err := os.Create(argsValue.OutputTar)
	if err != nil {
		panic(err)
	}
	defer saveFile.Close()

	client, err := container.GetDefaultContainerClient()
	if err != nil {
		panic(err)
	}

	err = container.ExportContainerRootfs(
		client, argsValue.ContainerId, saveFile)
	if err != nil {
		panic(err)
	}

	err = container.ExportAndPushContainerImageByRootfs(client, argsValue.ContainerId, container.PushRequest{
		ImageName:      "test:latest",
		RegisterDomain: "127.0.0.1:8000",
		Progress: func(progress container.Progress) error {
			slog.Info("dsfsf", "pro", progress)
			return nil
		},
	})
}
