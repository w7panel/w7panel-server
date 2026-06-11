//go:build linux

package cgroups

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	cgroup2 "github.com/containerd/cgroups/v3/cgroup2"
	"github.com/containerd/cgroups/v3/cgroup2/stats"
)

var currentcgroup string

func init() {
	path, err := os.ReadFile("/proc/1/cgroup")
	if err != nil {
		slog.Error("cannot open cgroup file", "error", err)
	}
	if err == nil {
		spath := string(path)
		spath = strings.ReplaceAll(spath, "\n", "")
		if strings.HasPrefix(spath, "0::/") {
			currentcgroup = spath[3:]
			currentcgroup = filepath.Dir(currentcgroup)
		}
	}
}

func Load(cgroupRoot string) (*cgroup2.Manager, error) {
	return cgroup2.Load(cgroupRoot)
}

func Current() (*cgroup2.Manager, error) {
	return Load(currentcgroup)
}

func CurrentStat() (*stats.Metrics, error) {
	cgroup, err := Load(currentcgroup)
	if err != nil {
		return nil, err
	}
	return cgroup.Stat()
}
