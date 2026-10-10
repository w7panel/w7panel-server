package controller

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenMergeDestinationUsesDirectoryOwnerAndMode(t *testing.T) {
	dir := t.TempDir()
	if os.Geteuid() == 0 {
		if err := os.Chown(dir, 33, 33); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0750); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "upload.tgz")
	file, err := openMergeDestination(path, true)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	parent, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := info.Sys().(*syscall.Stat_t)
	parentOwner := parent.Sys().(*syscall.Stat_t)
	if owner.Uid != parentOwner.Uid || owner.Gid != parentOwner.Gid {
		t.Fatalf("uploaded file owner %d:%d, want directory owner %d:%d", owner.Uid, owner.Gid, parentOwner.Uid, parentOwner.Gid)
	}
	if info.Mode().Perm() != 0750 {
		t.Fatalf("uploaded file mode = %v, want 0750", info.Mode())
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	file, err = openMergeDestination(path, true)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0777 {
		t.Fatalf("existing file mode = %v, want directory mode 0777", info.Mode())
	}
	updatedOwner := info.Sys().(*syscall.Stat_t)
	if updatedOwner.Uid != owner.Uid || updatedOwner.Gid != owner.Gid {
		t.Fatalf("existing file owner changed to %d:%d", updatedOwner.Uid, updatedOwner.Gid)
	}
}
