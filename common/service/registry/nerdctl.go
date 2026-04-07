package registry

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	// "github.com/containerd/containerd"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/commit"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/opencontainers/go-digest"
	cd "github.com/w7panel/w7panel/common/service/registry/containerd"
)

// commit a image
func Commit(ctx context.Context, client *containerd.Client, rawRef string, req string, options types.ContainerCommitOptions) error {
	return container.Commit(ctx, client, rawRef, req, options)
}

func Tag(ctx context.Context, client *containerd.Client, source, target string) error {
	options := types.ImageTagOptions{
		Source: source,
		Target: target,
	}
	options.GOptions = types.GlobalCommandOptions{
		Namespace: cd.NS,
	}
	return image.Tag(ctx, client, options)
}

func ImagesList(ctx context.Context, client *containerd.Client, filters, nameAndRefFilter []string) ([]images.Image, error) {
	return image.List(ctx, client, filters, nameAndRefFilter)
}

func ImagesRemove(ctx context.Context, client *containerd.Client, args []string, force bool, async bool) error {
	options := types.ImageRemoveOptions{
		Force:  force,
		Async:  async,
		Stdout: os.Stdout,
	}
	options.GOptions = types.GlobalCommandOptions{
		Namespace: cd.NS,
	}
	return image.Remove(ctx, client, args, options)
}

func ImagesLabel(ctx context.Context, client *containerd.Client, name string, labels map[string]string, replaceAll bool) error {
	var (
		is         = client.ImageService()
		fieldpaths []string
	)

	for k := range labels {
		if replaceAll {
			fieldpaths = append(fieldpaths, "labels")
		} else {
			fieldpaths = append(fieldpaths, strings.Join([]string{"labels", k}, "."))
		}
	}

	image := images.Image{
		Name:   name,
		Labels: labels,
	}
	cd.WithNamespace(ctx)
	_, err := is.Update(ctx, image, fieldpaths...)
	if err != nil {
		return err
	}

	return nil
}

// TODO 限定目录
func ImagesImportFromFile(ctx context.Context, client *containerd.Client, ref string, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return ImagesImport(ctx, client, ref, file)
}
func ImagesImport(ctx context.Context, client *containerd.Client, ref string, reader io.Reader) (string, error) {
	options := types.ImageImportOptions{
		// Source:    name,
		Stdin:     reader,
		Stdout:    os.Stdout,
		Reference: ref,
		GOptions: types.GlobalCommandOptions{
			Namespace:   cd.NS,
			Snapshotter: defaults.DefaultSnapshotter,
		},
	}

	return image.Import(ctx, client, options)
}

func CommitOne(ctx context.Context, client *containerd.Client, rawRef string, req string, options types.ContainerCommitOptions) (digest.Digest, error) {
	parsedReference, err := referenceutil.Parse(rawRef)
	if err != nil {
		return "", err
	}

	// changes, err := parseChanges(options.Change)
	// if err != nil {
	// 	return err
	// }

	opts := &commit.Opts{
		Author:             options.Author,
		Message:            options.Message,
		Ref:                parsedReference.String(),
		Pause:              options.Pause,
		Changes:            commit.Changes{},
		Compression:        options.Compression,
		Format:             options.Format,
		EstargzOptions:     options.EstargzOptions,
		ZstdChunkedOptions: options.ZstdChunkedOptions,
	}

	ctn, err := client.LoadContainer(ctx, req)
	if err != nil {
		return "", err
	}

	imageID, err := commit.Commit(ctx, client, ctn, opts, options.GOptions)
	if err != nil {
		return "", err
	}
	return imageID, nil
}

func Pull(ctx context.Context, client *containerd.Client, rawRef string, options types.ImagePullOptions) error {
	return image.Pull(ctx, client, rawRef, options)
}

func CommitToContainerD(ctx context.Context, rawRef, containerId string) (digest.Digest, error) {
	client, err := cd.CreateClient()
	if err != nil {
		return "", err
	}
	defer client.Close()
	return CommitOne(ctx, client, rawRef, containerId, types.ContainerCommitOptions{
		GOptions: types.GlobalCommandOptions{DataRoot: "/tmp", Address: cd.ContainerAddr()},
	})
}
func PullToContainerD(ctx context.Context, rawRef string, target string) error {
	client, err := cd.CreateClient()
	if err != nil {
		return err
	}
	defer client.Close()
	gOptions := types.GlobalCommandOptions{
		Namespace: cd.NS,
	}
	err = Pull(ctx, client, rawRef, types.ImagePullOptions{
		// Std
		Stdout:                 os.Stdout,
		Stderr:                 os.Stderr,
		ProgressOutputToStdout: true,
		Mode:                   "always",
		GOptions:               gOptions,
	})
	if err != nil {
		slog.Error("pull err", "err", err)
		return err
	}
	return Tag(ctx, client, rawRef, target)
}
