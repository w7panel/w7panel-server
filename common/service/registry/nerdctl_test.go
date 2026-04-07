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

func TestImport(t *testing.T) {
	os.Setenv("DEBUG", "true")
	client, err := cd.CreateClient()
	if err != nil {
		t.Log(err)
		return
	}
	ctx := context.Background()
	cd.WithNamespace(ctx)
	file, err := os.OpenFile("/tmp/test.tar", os.O_RDONLY, 0666)
	if err != nil {
		t.Log(err)
		return
	}
	defer file.Close()
	// ref := "registry.local.w7.cc/test/test:test"
	ref := ""
	dig, err := ImagesImport(context.Background(), client, ref, file)
	if err != nil {
		t.Log(err)
		return
	}
	t.Log(dig)
}
