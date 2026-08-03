// Package addon removes legacy K3s Addon sources that are now managed by the
// panel upgrade process. K3s continuously reconciles files in its manifests
// directory, so deleting the Kubernetes object alone is not sufficient.
package addon

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const manifestsRelativePath = "var/lib/rancher/k3s/server/manifests"

var (
	addonGVR     = schema.GroupVersionResource{Group: "k3s.cattle.io", Version: "v1", Resource: "addons"}
	helmChartGVR = schema.GroupVersionResource{Group: "helm.cattle.io", Version: "v1", Resource: "helmcharts"}

	legacyManifestFiles = []string{
		"cert-manager.yaml",
		"cert-manager.yml",
		"higress.yaml",
		"higress.yml",
	}
	legacyAddons = []string{"cert-manager", "higress"}
)

// IsK3sServer reports whether hostRoot belongs to a K3s server node. The
// k8s-offline DaemonSet runs on all nodes, while only server nodes own Addons.
func IsK3sServer(hostRoot string) bool {
	info, err := os.Stat(filepath.Join(hostRoot, manifestsRelativePath))
	return err == nil && info.IsDir()
}

// Cleanup removes legacy manifest sources and their K3s controller objects.
// It is safe to run repeatedly and is intentionally limited to Higress and
// cert-manager; all other K3s Addons remain untouched.
func Cleanup(ctx context.Context, client dynamic.Interface, hostRoot string) error {
	manifestDir := filepath.Join(hostRoot, manifestsRelativePath)
	if !IsK3sServer(hostRoot) {
		slog.Debug("skip legacy addon cleanup on non-server node", "manifestDir", manifestDir)
		return nil
	}

	for _, name := range legacyManifestFiles {
		path := filepath.Join(manifestDir, name)
		err := os.Remove(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove legacy k3s manifest %s: %w", path, err)
		}
		if err == nil {
			slog.Info("removed legacy k3s manifest", "path", path)
		}
	}

	for _, name := range legacyAddons {
		if err := client.Resource(addonGVR).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("delete legacy k3s addon %s: %w", name, err)
		}
		slog.Info("ensured legacy k3s addon is removed", "name", name)
	}

	// if err := client.Resource(helmChartGVR).Namespace("kube-system").Delete(ctx, "higress", metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
	// 	return fmt.Errorf("delete legacy k3s helmchart higress: %w", err)
	// }
	// slog.Info("ensured legacy k3s helmchart is removed", "namespace", "kube-system", "name", "higress")
	return nil
}
