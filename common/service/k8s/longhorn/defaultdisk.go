package longhorn

import (
	"errors"
	"fmt"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

const (
	defaultDiskLabel      = "node.longhorn.io/create-default-disk"
	defaultDiskAnnotation = "node.longhorn.io/default-disks-config"
	defaultDiskPending    = "w7panel.w7.com/default-disk-install-pending"
	defaultDiskConfig     = `[ { "path":"/var/lib/longhorn", "allowScheduling":true, "name": "disk-default", "tags":[ "disk-default" ] } ]`
)

func markDefaultDiskInstall(node *corev1.Node) bool {
	if node.Labels["node-role.kubernetes.io/control-plane"] != "true" {
		return false
	}
	if value := node.Labels[defaultDiskLabel]; value != "" && value != "config" {
		return false
	}
	if value := node.Annotations[defaultDiskAnnotation]; value != "" && value != defaultDiskConfig {
		return false
	}
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	node.Labels[defaultDiskLabel] = "config"
	node.Annotations[defaultDiskAnnotation] = defaultDiskConfig
	node.Annotations[defaultDiskPending] = "true"
	return true
}

func clearDefaultDiskInstall(node *corev1.Node, diskCreated, force bool) bool {
	if node.Labels[defaultDiskLabel] != "config" || node.Annotations[defaultDiskAnnotation] != defaultDiskConfig {
		return false
	}
	if !force && node.Annotations[defaultDiskPending] == "true" && !diskCreated {
		return false
	}
	delete(node.Labels, defaultDiskLabel)
	delete(node.Annotations, defaultDiskAnnotation)
	delete(node.Annotations, defaultDiskPending)
	return true
}

// PrepareDefaultDiskInstall labels only the control-plane nodes present at first install.
func PrepareDefaultDiskInstall(sdk *k8s.Sdk) ([]string, error) {
	nodes, err := sdk.ClientSet.CoreV1().Nodes().List(sdk.Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	marked := make([]string, 0)
	for _, item := range nodes.Items {
		if item.Labels["node-role.kubernetes.io/control-plane"] != "true" {
			continue
		}
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			node, err := sdk.ClientSet.CoreV1().Nodes().Get(sdk.Ctx, item.Name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !markDefaultDiskInstall(node) {
				return nil
			}
			_, err = sdk.ClientSet.CoreV1().Nodes().Update(sdk.Ctx, node, metav1.UpdateOptions{})
			return err
		})
		if err != nil {
			return marked, fmt.Errorf("mark node %s for Longhorn default disk: %w", item.Name, err)
		}
		marked = append(marked, item.Name)
	}
	return marked, nil
}

func clearDefaultDiskNode(sdk *k8s.Sdk, name string, diskCreated, force bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := sdk.ClientSet.CoreV1().Nodes().Get(sdk.Ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !clearDefaultDiskInstall(node, diskCreated, force) {
			return nil
		}
		_, err = sdk.ClientSet.CoreV1().Nodes().Update(sdk.Ctx, node, metav1.UpdateOptions{})
		return err
	})
}

func CancelDefaultDiskInstall(sdk *k8s.Sdk, names []string) error {
	var cleanupErr error
	for _, name := range names {
		if err := clearDefaultDiskNode(sdk, name, false, true); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("cleanup node %s: %w", name, err))
		}
	}
	return cleanupErr
}
