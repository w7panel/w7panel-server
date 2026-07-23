package console

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	if errors.Is(err, driver.ErrReleaseNotFound) {
		release, err = helmApi.Info("longhorn", longhornNamespace)
	}
	if err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) {
			slog.Info("longhorn helm 未安装，跳过迁移", "namespace", longhornNamespace)
			return
		}
		slog.Error("查询 longhorn helm 失败", "namespace", longhornNamespace, "err", err)
		os.Exit(1)
	}

	version := longhornVersion(release)
	if version == "" {
		slog.Error("longhorn version not found", "namespace", longhornNamespace, "releaseName", longhornReleaseName)
		os.Exit(1)
	}

	shouldRun, err := shouldRunLonghornUpgradeShell(sdk)
	if err != nil {
		slog.Error("check longhorn appgroup error", "err", err)
		os.Exit(1)
	}
	if !shouldRun {
		slog.Info("longhorn appgroup already exists, skip upgrade shell")
		return
	}

	koDataPath, ok := os.LookupEnv("KO_DATA_PATH")
	if !ok {
		koDataPath = "./kodata"
	}

	scriptPath := filepath.Join(koDataPath, "shell", "upgradelonghorn.sh")
	stdout, stderr, err := helper.Runsh("bash", scriptPath, version)
	if stdout != "" {
		slog.Info("longhorn upgrade shell stdout", "output", stdout)
	}
	if stderr != "" {
		slog.Warn("longhorn upgrade shell stderr", "output", stderr)
	}
	if err != nil {
		slog.Error("run longhorn upgrade shell error", "script", scriptPath, "version", version, "err", err)
		os.Exit(1)
	}

	slog.Info("longhorn upgrade shell success", "script", scriptPath, "version", version)
}

func shouldRunLonghornUpgradeShell(sdk *k8s.Sdk) (bool, error) {
	groupApi, err := appgroup.NewAppGroupApi(sdk)
	if err != nil {
		return false, err
	}

	for _, name := range []string{longhornReleaseName} {
		_, err := groupApi.GetAppGroup(sdk.GetNamespace(), name)
		if err == nil {
			return false, nil
		}
		if apierrors.IsNotFound(err) {
			return true, nil
		}
	}

	// groups, err := groupApi.GetAppGroupListByIdentifie("default", longhornReleaseName)
	// if err != nil {
	// 	return false, err
	// }
	return false, err
	// return len(groups.Items) == 0, nil
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
