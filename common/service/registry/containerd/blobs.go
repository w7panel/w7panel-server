package containerd

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/v2/core/content"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type containerdBlobHandler struct {
	store   content.Store
	putLock sync.Mutex
}

type blobNotFoundError struct {
	err error
}

func (e blobNotFoundError) Error() string {
	return e.err.Error()
}

func (e blobNotFoundError) Unwrap() error {
	return e.err
}

func (e blobNotFoundError) Is(target error) bool {
	return target != nil && target.Error() == "not found"
}

func NewBlobHandler(store content.Store) *containerdBlobHandler {
	return &containerdBlobHandler{
		store: store,
	}
}

func (handler *containerdBlobHandler) Get(ctx context.Context, repo string, h v1.Hash) (io.ReadCloser, error) {
	dgst, err := digest.Parse(h.String())
	if err != nil {
		return nil, err
	}
	body, err := content.ReadBlob(WithNamespace(ctx), handler.store, ocispec.Descriptor{Digest: dgst})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(body)), nil
	// return nil, errors.New("not impl")
}

// blobs 是否已存在
func (hd *containerdBlobHandler) Stat(ctx context.Context, _ string, h v1.Hash) (int64, error) {
	dgst, err := digest.Parse(h.String())
	if err != nil {
		return 0, err
	}
	info, err := hd.store.Info(WithNamespace(ctx), dgst)
	if errdefs.IsNotFound(err) {
		return 0, blobNotFoundError{err: err}
	}
	if err != nil {
		return 0, err
	}
	return info.Size, nil
}

func (hd *containerdBlobHandler) Put(ctx context.Context, repo string, h v1.Hash, rc io.ReadCloser) error {
	hd.putLock.Lock()
	defer hd.putLock.Unlock()
	// v1.Hash 转 digest
	dgst, err := digest.Parse(h.String())
	if err != nil {
		return err
	}
	ctx = WithNamespace(ctx)

	// 读取所有数据以便获取大小
	body, err := io.ReadAll(rc)
	if err != nil {
		return err
	}

	desc := ocispec.Descriptor{
		Digest: dgst,
		Size:   int64(len(body)),
	}
	if err := content.WriteBlob(ctx, hd.store, dgst.String(), bytes.NewReader(body), desc); err != nil && !errdefs.IsAlreadyExists(err) {
		return err
	}
	return nil
}
