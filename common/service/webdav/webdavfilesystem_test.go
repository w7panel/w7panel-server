package webdav2

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/net/webdav"
)

type renameEXDEVFS struct {
	webdav.Dir
}

func (fs renameEXDEVFS) Rename(ctx context.Context, oldName, newName string) error {
	return &os.LinkError{Op: "rename", Old: oldName, New: newName, Err: syscall.EXDEV}
}

func TestWebDAVFileSystemCreateUsesParentPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	parentPath := filepath.Join(tmpDir, "parent")
	if err := os.Mkdir(parentPath, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parentPath, 0o750); err != nil {
		t.Fatal(err)
	}

	fs := NewWebDAVFileSystem(webdav.Dir(tmpDir), tmpDir)
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/parent/new-dir", 0o777); err != nil {
		t.Fatal(err)
	}

	file, err := fs.OpenFile(ctx, "/parent/new-file", os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"new-dir", "new-file"} {
		got, err := os.Stat(filepath.Join(parentPath, name))
		if err != nil {
			t.Fatal(err)
		}
		wantPath := filepath.Join(tmpDir, "expected-"+name)
		var wantErr error
		if name == "new-dir" {
			wantErr = os.Mkdir(wantPath, 0o750)
		} else {
			var wantFile *os.File
			wantFile, wantErr = os.OpenFile(wantPath, os.O_CREATE|os.O_WRONLY, 0o750)
			if wantErr == nil {
				wantErr = wantFile.Close()
			}
		}
		if wantErr != nil {
			t.Fatal(wantErr)
		}
		want, err := os.Stat(wantPath)
		if err != nil {
			t.Fatal(err)
		}
		if got.Mode().Perm() != want.Mode().Perm() {
			t.Errorf("%s permissions = %04o, want %04o", name, got.Mode().Perm(), want.Mode().Perm())
		}
	}
}

func TestWebDAVFileSystemOverwriteKeepsExistingPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "parent"), 0o750); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(tmpDir, "parent", "existing")
	if err := os.WriteFile(name, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0o600); err != nil {
		t.Fatal(err)
	}
	fs := NewWebDAVFileSystem(webdav.Dir(tmpDir), tmpDir)
	file, err := fs.OpenFile(context.Background(), "/parent/existing", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o777)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("existing file permissions = %04o, want 0600", info.Mode().Perm())
	}
}

func TestWebDAVFileSystemRename_FallbackCopiesAndRemovesFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webdav-rename-file")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	srcRel := "/src.txt"
	dstRel := "/dst.txt"
	srcAbs := filepath.Join(tmpDir, "src.txt")
	dstAbs := filepath.Join(tmpDir, "dst.txt")

	if err := os.WriteFile(srcAbs, []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}

	fs := NewWebDAVFileSystem(renameEXDEVFS{Dir: webdav.Dir(tmpDir)}, tmpDir)
	if err := fs.Rename(context.Background(), srcRel, dstRel); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(srcAbs); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source should be removed, got err=%v", err)
	}
	data, err := os.ReadFile(dstAbs)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected destination content: %q", string(data))
	}
	info, err := os.Stat(dstAbs)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("unexpected destination perm: %o", info.Mode().Perm())
	}
}

func TestWebDAVFileSystemRename_FallbackCopiesAndRemovesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webdav-rename-dir")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	srcDirRel := "/srcdir"
	dstDirRel := "/dstdir"
	srcDirAbs := filepath.Join(tmpDir, "srcdir")
	dstDirAbs := filepath.Join(tmpDir, "dstdir")

	if err := os.MkdirAll(filepath.Join(srcDirAbs, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDirAbs, "nested", "file.txt"), []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := NewWebDAVFileSystem(renameEXDEVFS{Dir: webdav.Dir(tmpDir)}, tmpDir)
	if err := fs.Rename(context.Background(), srcDirRel, dstDirRel); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(srcDirAbs); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source dir should be removed, got err=%v", err)
	}
	data, err := os.ReadFile(filepath.Join(dstDirAbs, "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested" {
		t.Fatalf("unexpected nested content: %q", string(data))
	}
}

func TestWebDAVFileSystemRename_FallbackCopiesSymlink(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "webdav-rename-symlink")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	targetAbs := filepath.Join(tmpDir, "target.txt")
	if err := os.WriteFile(targetAbs, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}

	srcRel := "/link"
	dstRel := "/link2"
	srcAbs := filepath.Join(tmpDir, "link")
	dstAbs := filepath.Join(tmpDir, "link2")
	if err := os.Symlink("target.txt", srcAbs); err != nil {
		t.Fatal(err)
	}

	fs := NewWebDAVFileSystem(renameEXDEVFS{Dir: webdav.Dir(tmpDir)}, tmpDir)
	if err := fs.Rename(context.Background(), srcRel, dstRel); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(srcAbs); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source symlink should be removed, got err=%v", err)
	}
	info, err := os.Lstat(dstAbs)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("destination should remain symlink")
	}
	target, err := os.Readlink(dstAbs)
	if err != nil {
		t.Fatal(err)
	}
	if target != "target.txt" {
		t.Fatalf("unexpected symlink target: %s", target)
	}
}
