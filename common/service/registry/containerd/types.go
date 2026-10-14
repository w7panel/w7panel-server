package containerd

import (
	"context"

	v2client "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/w7panel/w7panel/common/helper"
)

const (
	NS = "k8s.io"
	// containerdRoot    = "/run/k3s/containerd"
	// containerdAddr    = "/run/k3s/containerd/containerd.sock"
	REPO                = "registry.local.w7.cc"
	debugcontainerdRoot = "/var/lib/containerd"
	debugcontainerdAddr = "/run/containerd/containerd.sock"
	k3sContainerAddr    = "/var/run/k3s/containerd/containerd.sock"
	k3sContainerRoot    = "/var/lib/rancher/k3s/agent/containerd"
)

type tagMetadata struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

func WithNamespace(ctx context.Context) context.Context {
	return namespaces.WithNamespace(ctx, NS)
}

func CreateClient() (*v2client.Client, error) {
	client, err := v2client.New(ContainerAddr(), v2client.WithDefaultNamespace(NS))
	if err != nil {
		return nil, err
	}
	return client, err
}

func ContainerAddr() string {
	if helper.IsLocalMock() || helper.IsDebug() {
		return debugcontainerdAddr
	}
	// CKM child panels run the same registry daemon but are identified with
	// IS_CHILD rather than IS_AGENT. Both pods mount the k3s containerd socket
	// at the same path.
	if helper.IsAgent() || helper.IsChildAgent() {
		return k3sContainerAddr
	}

	return ""

}
