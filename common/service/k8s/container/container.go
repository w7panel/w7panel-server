package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/mount"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/w7panel/w7panel/common/helper"
)

var defaultRootFSExcludePrefixes = []string{
	".dockerenv",
	"dev",
	"proc",
	"sys",
	"run/secrets",
	"var/run/secrets",
}

const socketPath = "/run/k3s/containerd/containerd.sock"

func GetDefaultContainerClient() (*containerd.Client, error) {
	containedClient, err := containerd.New(socketPath, containerd.WithDefaultNamespace("k8s.io"))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %v", err)
	}

	return containedClient, nil
}

func ExportContainerRootfs(containedClient *containerd.Client, containerID string, writer io.Writer) error {
	ctx := namespaces.WithNamespace(context.Background(), "k8s.io")

	container, err := containedClient.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	snapshots := containedClient.SnapshotService("overlayfs")
	mounts, err := snapshots.Mounts(ctx, container.ID())
	if err != nil {
		return fmt.Errorf("get mounts failed (container may not be running): %w", err)
	}
	if len(mounts) == 0 {
		return errors.New("no mount info found, container may not be running")
	}
	slog.Info("mount info", "mounts", mounts)

	if err := exportMountedRootFSTar(mounts, writer); err != nil {
		return fmt.Errorf("export container rootfs failed: %w", err)
	}

	return nil
}

func exportMountedRootFSTar(mounts []mount.Mount, writer io.Writer) error {
	return mount.WithTempMount(context.Background(), mounts, func(root string) error {
		if err := helper.EnsureDirExists(root); err != nil {
			return err
		}
		return helper.CreateTarFromDirToWriter(root, writer, shouldExcludeRootFSPath)
	})
}

func shouldExcludeRootFSPath(relPath string, info os.FileInfo) bool {
	cleanPath := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	cleanPath = strings.TrimPrefix(cleanPath, "/")
	if cleanPath == "" {
		return false
	}

	for _, prefix := range defaultRootFSExcludePrefixes {
		if cleanPath == prefix || strings.HasPrefix(cleanPath, prefix+"/") {
			if info.IsDir() {
				slog.Debug("skip rootfs dir from flatten image", "path", cleanPath)
			}
			return true
		}
	}

	return false
}
