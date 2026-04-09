package container

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/compression"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/w7panel/w7panel/common/helper"
)

func ExportAndPushContainerImageByRootfs(containedClient *containerd.Client, containerId string, pushReq PushRequest) error {
	ctx := namespaces.WithNamespace(context.Background(), "k8s.io")
	container, err := containedClient.LoadContainer(ctx, containerId)
	if err != nil {
		return fmt.Errorf("load container failed: %w", err)
	}
	containerInfo, err := container.Info(ctx)
	if err != nil {
		return fmt.Errorf("get container info failed: %w", err)
	}
	var localBaseImage containerd.Image
	var localImageErr error
	if containerInfo.Image != "" {
		localBaseImage, localImageErr = containedClient.GetImage(ctx, containerInfo.Image)
	}
	targetRef, err := parseRegistryReference(pushReq.RegisterDomain+"/"+pushReq.ImageName, pushReq.RegistryPlainHTTP)
	if err != nil {
		return err
	}

	slog.Info("exporting container image", "containerId", containerId)

	if pushReq.Progress != nil {
		err = pushReq.Progress(Progress{
			Content: fmt.Sprintf("exporting container image, containerId: %s", containerId),
		})
		if err != nil {
			return err
		}
	}

	rootfsBuffer := bytes.NewBuffer(nil)
	if err := ExportContainerRootfs(containedClient, containerId, rootfsBuffer); err != nil {
		return fmt.Errorf("export container rootfs failed: %w", err)
	}

	slog.Info("push image to container registry", "registry", pushReq.RegisterDomain, "image", pushReq.ImageName)

	if pushReq.Progress != nil {
		err = pushReq.Progress(Progress{
			Content: fmt.Sprintf("push image to container registry, containerId: %s, image: %s, registry: %s", containerId, pushReq.ImageName, pushReq.RegisterDomain),
		})
		if err != nil {
			return err
		}
	}

	layer, err := tarball.LayerFromOpener(bufferOpener(rootfsBuffer.Bytes()),
		tarball.WithCompression(compression.GZip),
		tarball.WithCompressedCaching,
		tarball.WithMediaType(types.OCILayer),
	)
	if err != nil {
		return fmt.Errorf("load rootfs layer tar failed: %w", err)
	}

	image, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer: layer,
		History: v1.History{
			Author:    "w7panel",
			CreatedBy: "w7panel flatten rootfs",
			Comment:   "single-layer image from mounted container rootfs",
		},
		MediaType: types.OCILayer,
	})
	if err != nil {
		return fmt.Errorf("append rootfs layer failed: %w", err)
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		return fmt.Errorf("read generated image config failed: %w", err)
	}
	configFile.Architecture = runtime.GOARCH
	configFile.OS = "linux"
	configFile.Created = v1.Time{Time: time.Now().UTC()}
	if localImageErr == nil {
		if baseSpec, specErr := localBaseImage.Spec(ctx); specErr == nil {
			inheritRuntimeConfigFromOCI(configFile, &baseSpec)
		}
	}

	image, err = mutate.ConfigFile(image, configFile)
	if err != nil {
		return fmt.Errorf("update image config failed: %w", err)
	}
	image = mutate.MediaType(image, types.OCIManifestSchema1)
	image = mutate.ConfigMediaType(image, types.OCIConfigJSON)

	if err := remote.Write(targetRef, image, remoteOptions(pushReq)...); err != nil {
		return fmt.Errorf("push image to %q failed: %w", pushReq.ImageName, err)
	}

	return nil
}

func parseRegistryReference(ref string, plainHTTP bool) (name.Reference, error) {
	if ref == "" {
		return nil, fmt.Errorf("registry reference is empty")
	}

	opts := make([]name.Option, 0)
	if plainHTTP {
		opts = append(opts, name.Insecure)
	}

	parsedRef, err := name.ParseReference(ref, opts...)
	if err != nil {
		return nil, fmt.Errorf("parse registry reference %q failed: %w", ref, err)
	}
	return parsedRef, nil
}

func inheritRuntimeConfigFromOCI(dst *v1.ConfigFile, src *ocispec.Image) {
	if dst == nil || src == nil {
		return
	}

	dst.Author = src.Author
	dst.Config = v1.Config{
		Cmd:          append([]string(nil), src.Config.Cmd...),
		Entrypoint:   append([]string(nil), src.Config.Entrypoint...),
		Env:          append([]string(nil), src.Config.Env...),
		Labels:       helper.CloneMap(src.Config.Labels),
		User:         src.Config.User,
		Volumes:      helper.CloneMap(src.Config.Volumes),
		WorkingDir:   src.Config.WorkingDir,
		ExposedPorts: helper.CloneMap(src.Config.ExposedPorts),
		ArgsEscaped:  src.Config.ArgsEscaped,
		StopSignal:   src.Config.StopSignal,
	}
	dst.OSVersion = src.OSVersion
	dst.OSFeatures = append([]string(nil), src.OSFeatures...)
	dst.Variant = src.Variant
	if src.Architecture != "" {
		dst.Architecture = src.Architecture
	}
	if src.OS != "" {
		dst.OS = src.OS
	}
}

func remotePushProgress(progress func(Progress) error) remote.Option {
	updates := make(chan v1.Update, 128)
	go func() {
		defer close(updates)
		for update := range updates {
			info := fmt.Sprintf("push image status: complete: %d, total: %d, err: %s", update.Complete, update.Total, update.Error)

			if progress != nil {
				err := progress(Progress{
					Content: info,
				})
				if err != nil {
					return
				}
			}

			if update.Total > 0 && update.Complete >= update.Total {
				return
			}
		}
	}()

	return remote.WithProgress(updates)
}

func remoteOptions(req PushRequest) []remote.Option {
	defaultOptions := make([]remote.Option, 0)
	if req.Progress != nil {
		defaultOptions = append(defaultOptions, remotePushProgress(req.Progress))
	}

	if req.RegistryUsername != "" || req.RegistryPassword != "" {
		return append(defaultOptions, remote.WithAuth(authn.FromConfig(authn.AuthConfig{
			Username: req.RegistryUsername,
			Password: req.RegistryPassword,
		})),
		)
	}

	return append(defaultOptions, remote.WithAuthFromKeychain(authn.DefaultKeychain))
}

func bufferOpener(content []byte) tarball.Opener {
	return func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), nil
	}
}
