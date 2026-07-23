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
	"github.com/containerd/containerd/v2/pkg/namespaces"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
)

var defaultRootFSExcludePrefixes = []string{
	".dockerenv",
	"dev",
	"proc",
	"sys",
	"run/secrets",
	"var/run/secrets",
}

const socketPath = "/var/run/k3s/containerd/containerd.sock"

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
	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("load container task failed (container may not be running): %w", err)
	}
	spec, err := task.Spec(ctx)
	if err != nil {
		return fmt.Errorf("load container task spec failed: %w", err)
	}
	if spec == nil {
		return errors.New("container task spec is empty")
	}

	rootfsPath := getContainerRootfsPath(containerID, spec)
	if err := exportContainerRootfs(rootfsPath, spec.Mounts, writer); err != nil {
		return fmt.Errorf("export container rootfs failed: %w", err)
	}

	return nil
}

func exportContainerRootfs(rootfsPath string, mounts []specs.Mount, writer io.Writer) error {
	info, err := os.Stat(rootfsPath)
	if err != nil {
		return fmt.Errorf("stat container rootfs failed: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("container rootfs path is not directory: %s", rootfsPath)
	}

	excludedMounts := getExcludedMountPaths(mounts)

	slog.Info("export container rootfs", "rootfsPath", rootfsPath, "excludedMounts", excludedMounts)

	return helper.CreateTarFromDirToWriter(rootfsPath, writer, func(relPath string, info os.FileInfo) bool {
		return shouldExcludeRootFSPath(relPath, info, excludedMounts)
	})
}

func getContainerRootfsPath(containerID string, spec *specs.Spec) string {
	if spec != nil && spec.Root != nil && spec.Root.Path != "" {
		if filepath.IsAbs(spec.Root.Path) {
			return spec.Root.Path
		}
		return filepath.Join(getContainerBundlePath(containerID), spec.Root.Path)
	}
	return filepath.Join(getContainerBundlePath(containerID), "rootfs")
}

func getContainerBundlePath(containerID string) string {
	return filepath.Join(facade.Config.GetString("k3s-registry.runtime_dir"), normalizeContainerID(containerID))
}

func normalizeContainerID(containerID string) string {
	return strings.TrimPrefix(containerID, "containerd://")
}

func getExcludedMountPaths(mounts []specs.Mount) map[string]struct{} {
	excluded := make(map[string]struct{})
	for _, mount := range mounts {
		mountPoint := normalizeMountPath(mount.Destination)
		if mountPoint == "" || mountPoint == "." {
			continue
		}
		excluded[mountPoint] = struct{}{}
	}
	return excluded
}

func normalizeMountPath(path string) string {
	path = strings.ReplaceAll(path, `\040`, " ")
	path = strings.ReplaceAll(path, `\011`, "\t")
	path = strings.ReplaceAll(path, `\012`, "\n")
	path = strings.ReplaceAll(path, `\134`, `\`)
	path = filepath.ToSlash(strings.TrimPrefix(path, "/"))
	path = strings.Trim(path, "/")
	return path
}

func shouldExcludeRootFSPath(relPath string, info os.FileInfo, excludedMounts map[string]struct{}) bool {
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

	for mountPath := range excludedMounts {
		if cleanPath == mountPath || strings.HasPrefix(cleanPath, mountPath+"/") {
			if info.IsDir() {
				slog.Debug("skip mounted dir from flatten image", "path", cleanPath, "mountPath", mountPath)
			} else {
				slog.Debug("skip mounted file from flatten image", "path", cleanPath, "mountPath", mountPath)
			}
			return true
		}
	}

	return false
}
