package helper

import (
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func CopyRecursive(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return CopyDir(src, dst)
	}
	return CopyFile(src, dst)
}

// copyDir recursively copies a directory
func CopyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// copyFile copies a single file
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

func GetXattr(path, attr string) (string, error) {
	size, err := unix.Getxattr(path, attr, nil)
	if err != nil {
		return "", err
	}
	if size == 0 {
		return "", nil
	}

	buf := make([]byte, size)
	n, err := unix.Getxattr(path, attr, buf)
	if err != nil {
		return "", err
	}

	return string(buf[:n]), nil
}

func ReadXattrs(path string, allow func(string) bool) (map[string]string, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENODATA) || errors.Is(err, unix.EPERM) {
			return nil, nil
		}
		return nil, err
	}
	if size <= 0 {
		return nil, nil
	}

	buf := make([]byte, size)
	n, err := unix.Listxattr(path, buf)
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENODATA) || errors.Is(err, unix.EPERM) {
			return nil, nil
		}
		return nil, err
	}

	result := make(map[string]string)
	start := 0
	for i := 0; i < n; i++ {
		if buf[i] != 0 {
			continue
		}
		if i == start {
			start = i + 1
			continue
		}
		key := string(buf[start:i])
		start = i + 1
		if allow != nil && !allow(key) {
			continue
		}
		value, err := GetXattr(path, key)
		if err != nil {
			if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENODATA) || errors.Is(err, unix.EPERM) {
				continue
			}
			return nil, err
		}
		result[key] = value
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}
