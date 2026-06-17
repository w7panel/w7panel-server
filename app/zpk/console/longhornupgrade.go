package console

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"helm.sh/helm/v3/pkg/release"
)

const (
	longhornReleaseName = "w7panel-longhorn" //应该是w7panel-longhorn 之前是longhorn有问题
	longhornNamespace   = "longhorn-system"
)

type LonghornUpgrade struct {
	console2.Abstract
}

func (c LonghornUpgrade) GetName() string {
	return "longhornupgrade"
}

func (c LonghornUpgrade) Handle(cmd *cobra.Command, args []string) {
	sdk := k8s.NewK8sClient().Sdk
	helmApi := k8s.NewHelm(sdk)

	release, err := helmApi.Info(longhornReleaseName, longhornNamespace)
	if err != nil {
		slog.Error("longhorn helm not found", "namespace", longhornNamespace, "releaseName", longhornReleaseName, "err", err)
		return
	}

	version := longhornVersion(release)
	if version == "" {
		slog.Error("longhorn version not found", "namespace", longhornNamespace, "releaseName", longhornReleaseName)
		return
	}

	koDataPath, ok := os.LookupEnv("KO_DATA_PATH")
	if !ok {
		koDataPath = "./kodata"
	}

	scriptPath := filepath.Join(koDataPath, "shell", "upgradelonghorn.sh")
	stdout, stderr, err := helper.Runsh("sh", scriptPath, version)
	if stdout != "" {
		slog.Info("longhorn upgrade shell stdout", "output", stdout)
	}
	if stderr != "" {
		slog.Warn("longhorn upgrade shell stderr", "output", stderr)
	}
	if err != nil {
		slog.Error("run longhorn upgrade shell error", "script", scriptPath, "version", version, "err", err)
		return
	}

	slog.Info("longhorn upgrade shell success", "script", scriptPath, "version", version)
}

func longhornVersion(release *release.Release) string {
	if release == nil || release.Chart == nil || release.Chart.Metadata == nil {
		return ""
	}

	version := release.Chart.Metadata.AppVersion
	if version == "" {
		version = release.Chart.Metadata.Version
	}
	return strings.ReplaceAll(version, "v", "")
}
