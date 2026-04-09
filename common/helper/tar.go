package helper

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func AppendTarHeaderXattrs(path string, header *tar.Header, allow func(string) bool) error {
	if header == nil {
		return nil
	}

	xattrs, err := ReadXattrs(path, allow)
	if err != nil {
		return err
	}
	if len(xattrs) == 0 {
		return nil
	}

	header.Format = tar.FormatPAX
	if header.PAXRecords == nil {
		header.PAXRecords = make(map[string]string, len(xattrs))
	}
	for key, value := range xattrs {
		header.PAXRecords[key] = value
	}
	return nil
}

func CreateTarFromDirToWriter(srcDir string, writer io.Writer, exclude func(string, os.FileInfo) bool) error {
	tw := tar.NewWriter(writer)
	defer tw.Close()
	hardlinks := make(map[string]string)
	usernames := make(map[uint32]string)
	groupnames := make(map[uint32]string)

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relPath)
		if exclude != nil && exclude(name, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return tw.WriteHeader(&tar.Header{
				Name:     name,
				Linkname: linkTarget,
				Typeflag: tar.TypeSymlink,
				Mode:     int64(info.Mode().Perm()),
			})
		}

		if info.IsDir() {
			header := &tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     int64(info.Mode().Perm()),
			}
			if stat, ok := info.Sys().(*unix.Stat_t); ok {
				header.Uid = int(stat.Uid)
				header.Gid = int(stat.Gid)
				//这两个在 uid namespace 不隔离的情况下可用， 隔离后得找到 uid namespace 然后再获取
				header.Uname = lookupUsername(stat.Uid, usernames)
				header.Gname = lookupGroupname(stat.Gid, groupnames)
			}
			if err := AppendTarHeaderXattrs(path, header, ShouldIncludeArchiveXattr); err != nil {
				return err
			}
			return tw.WriteHeader(header)
		}

		if info.Mode()&(os.ModeNamedPipe|os.ModeDevice|os.ModeCharDevice) != 0 {
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			if stat, ok := info.Sys().(*unix.Stat_t); ok {
				header.Uid = int(stat.Uid)
				header.Gid = int(stat.Gid)
				header.Uname = lookupUsername(stat.Uid, usernames)
				header.Gname = lookupGroupname(stat.Gid, groupnames)
				header.Devmajor = int64(unix.Major(uint64(stat.Rdev)))
				header.Devminor = int64(unix.Minor(uint64(stat.Rdev)))
			}
			header.Name = name
			if err := AppendTarHeaderXattrs(path, header, ShouldIncludeArchiveXattr); err != nil {
				return err
			}
			return tw.WriteHeader(header)
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		if stat, ok := info.Sys().(*unix.Stat_t); ok {
			header.Uid = int(stat.Uid)
			header.Gid = int(stat.Gid)
			header.Uname = lookupUsername(stat.Uid, usernames)
			header.Gname = lookupGroupname(stat.Gid, groupnames)
			if stat.Nlink > 1 {
				key := fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
				if firstPath, found := hardlinks[key]; found {
					header.Typeflag = tar.TypeLink
					header.Linkname = firstPath
					header.Size = 0
					header.Name = name
					return tw.WriteHeader(header)
				}
				hardlinks[key] = name
			}
		}
		header.Name = name
		if err := AppendTarHeaderXattrs(path, header, ShouldIncludeArchiveXattr); err != nil {
			return err
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		inFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer inFile.Close()

		_, err = io.Copy(tw, inFile)
		return err
	})
}

func lookupUsername(uid uint32, cache map[uint32]string) string {
	if cache != nil {
		if value, ok := cache[uid]; ok {
			return value
		}
	}

	name := strconv.FormatUint(uint64(uid), 10)
	if usr, err := user.LookupId(name); err == nil && usr != nil && usr.Username != "" {
		name = usr.Username
	}
	if cache != nil {
		cache[uid] = name
	}
	return name
}

func lookupGroupname(gid uint32, cache map[uint32]string) string {
	if cache != nil {
		if value, ok := cache[gid]; ok {
			return value
		}
	}

	name := strconv.FormatUint(uint64(gid), 10)
	if grp, err := user.LookupGroupId(name); err == nil && grp != nil && grp.Name != "" {
		name = grp.Name
	}
	if cache != nil {
		cache[gid] = name
	}
	return name
}

func ShouldIncludeArchiveXattr(key string) bool {
	switch {
	case strings.HasPrefix(key, "trusted.overlay."):
		return false
	case strings.HasPrefix(key, "user.overlay."):
		return false
	default:
		return true
	}
}
