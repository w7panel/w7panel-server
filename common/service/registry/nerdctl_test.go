package registry

import (
	"context"
	"os"
	"testing"

	cd "github.com/w7panel/w7panel/common/service/registry/containerd"
)

func TestNerdCommit(t *testing.T) {
	os.Setenv("DEBUG", "true")

	dig, err := CommitToContainerD(context.Background(), "ccr.ccs.tencentyun.com/afan-public/nginx:test", "nginx-test")
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(dig)
}

func TestImagesList(t *testing.T) {
	os.Setenv("DEBUG", "true")
	client, err := cd.CreateClient()
	if err != nil {
		t.Log(err)
		return
	}
	ctx := context.Background()
	cd.WithNamespace(ctx)
	dig, err := ImagesList(context.Background(), client, []string{}, []string{})
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(dig)
}
