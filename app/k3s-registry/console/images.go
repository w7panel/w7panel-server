package console

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/registry"
	cd "github.com/w7panel/w7panel/common/service/registry/containerd"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type ImagesList struct{ console2.Abstract }

func (ImagesList) GetName() string        { return "registry:image:list" }
func (ImagesList) GetDescription() string { return "list container images" }

func (ImagesList) Handle(cmd *cobra.Command, _ []string) {
	client, err := cd.CreateClient()
	if err != nil {
		commandError(cmd, "create containerd client", err)
		return
	}
	defer client.Close()

	images, err := registry.ImagesList(context.Background(), client, []string{"dangling=false"}, nil)
	if err != nil {
		commandError(cmd, "list images", err)
		return
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(images); err != nil {
		commandError(cmd, "write image list", err)
	}
}

type ImagesTag struct{ console2.Abstract }

type imagesTagOptions struct {
	Source string
	Target string
}

var imageTagOptions imagesTagOptions

func (ImagesTag) GetName() string        { return "registry:image:tag" }
func (ImagesTag) GetDescription() string { return "tag a container image" }

func (ImagesTag) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&imageTagOptions.Source, "source", "", "source image reference")
	cmd.Flags().StringVar(&imageTagOptions.Target, "target", "", "target image reference")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("target")
}

func (ImagesTag) Handle(cmd *cobra.Command, _ []string) {
	client, err := cd.CreateClient()
	if err != nil {
		commandError(cmd, "create containerd client", err)
		return
	}
	defer client.Close()

	if err := registry.Tag(context.Background(), client, imageTagOptions.Source, imageTagOptions.Target); err != nil {
		commandError(cmd, "tag image", err)
		return
	}
	cmd.Println("image tagged")
}

type ImagesRemove struct{ console2.Abstract }

type imagesRemoveOptions struct {
	Target string
	Force  bool
	Async  bool
}

var imageRemoveOptions imagesRemoveOptions

func (ImagesRemove) GetName() string        { return "registry:image:remove" }
func (ImagesRemove) GetDescription() string { return "remove a container image" }

func (ImagesRemove) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&imageRemoveOptions.Target, "target", "", "image reference to remove")
	cmd.Flags().BoolVar(&imageRemoveOptions.Force, "force", false, "force image removal")
	cmd.Flags().BoolVar(&imageRemoveOptions.Async, "async", false, "remove asynchronously")
	_ = cmd.MarkFlagRequired("target")
}

func (ImagesRemove) Handle(cmd *cobra.Command, _ []string) {
	client, err := cd.CreateClient()
	if err != nil {
		commandError(cmd, "create containerd client", err)
		return
	}
	defer client.Close()

	if err := registry.ImagesRemove(context.Background(), client, []string{imageRemoveOptions.Target}, imageRemoveOptions.Force, imageRemoveOptions.Async); err != nil {
		commandError(cmd, "remove image", err)
		return
	}
	cmd.Println("image removed")
}

type ImagesLabel struct{ console2.Abstract }

type imagesLabelOptions struct {
	Name    string
	Labels  []string
	Replace bool
}

var imageLabelOptions imagesLabelOptions

func (ImagesLabel) GetName() string        { return "registry:image:label" }
func (ImagesLabel) GetDescription() string { return "set container image labels" }

func (ImagesLabel) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&imageLabelOptions.Name, "name", "", "image reference")
	cmd.Flags().StringArrayVar(&imageLabelOptions.Labels, "label", nil, "label in key=value form; repeatable")
	cmd.Flags().BoolVar(&imageLabelOptions.Replace, "replace", false, "replace all existing labels")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("label")
}

func (ImagesLabel) Handle(cmd *cobra.Command, _ []string) {
	labels, err := parseLabels(imageLabelOptions.Labels)
	if err != nil {
		commandError(cmd, "parse labels", err)
		return
	}

	client, err := cd.CreateClient()
	if err != nil {
		commandError(cmd, "create containerd client", err)
		return
	}
	defer client.Close()

	if err := registry.ImagesLabel(context.Background(), client, imageLabelOptions.Name, labels, imageLabelOptions.Replace); err != nil {
		commandError(cmd, "label image", err)
		return
	}
	cmd.Println("image labels updated")
}

type ImagesImport struct{ console2.Abstract }

type imagesImportOptions struct {
	Name string
	Path string
}

var imageImportOptions imagesImportOptions

func (ImagesImport) GetName() string        { return "registry:image:import" }
func (ImagesImport) GetDescription() string { return "import a container image archive" }

func (ImagesImport) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&imageImportOptions.Name, "name", "", "image reference")
	cmd.Flags().StringVar(&imageImportOptions.Path, "path", "", "path to the image archive")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("path")
}

func (ImagesImport) Handle(cmd *cobra.Command, _ []string) {
	client, err := cd.CreateClient()
	if err != nil {
		commandError(cmd, "create containerd client", err)
		return
	}
	defer client.Close()

	imageName, err := registry.ImagesImportFromFile(context.Background(), client, imageImportOptions.Name, importPath(imageImportOptions.Path))
	if err != nil {
		commandError(cmd, "import image", err)
		return
	}
	cmd.Println(imageName)
}

func parseLabels(values []string) (map[string]string, error) {
	labels := make(map[string]string, len(values))
	for _, value := range values {
		key, labelValue, found := strings.Cut(value, "=")
		if !found || key == "" {
			return nil, fmt.Errorf("label %q must use key=value form", value)
		}
		labels[key] = labelValue
	}
	return labels, nil
}

func importPath(path string) string {
	if helper.IsAgent() || helper.IsK3kVirtual() {
		return filepath.Join(facade.GetConfig().GetString("s3.base_dir"), path)
	}
	return path
}

func commandError(cmd *cobra.Command, operation string, err error) {
	slog.Error(operation, "error", err)
	cmd.PrintErrf("%s: %v\n", operation, err)
	os.Exit(1)
}
