package console

import (
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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	higressReleaseName = "higress"
	higressGroupName   = "w7panel-higress"
	higressNamespace   = "higress-system"
)

type HigressUpgrade struct {
	console2.Abstract
}

func (c HigressUpgrade) GetName() string {
	return "higressupgrade"
}

func (c HigressUpgrade) Handle(cmd *cobra.Command, args []string) {
	sdk := k8s.NewK8sClient().Sdk
	helmApi := k8s.NewHelm(sdk)

	release, err := helmApi.Info(higressReleaseName, higressNamespace)
	if err != nil {
		slog.Error("higress helm not found", "namespace", higressNamespace, "releaseName", higressReleaseName, "err", err)
		return
	}

	version := higressVersion(release)
	if version == "" {
		slog.Warn("higress version not found", "namespace", higressNamespace, "releaseName", higressReleaseName)
	}

	shouldRun, err := shouldRunHigressUpgradeShell(sdk)
	if err != nil {
		slog.Error("check higress appgroup error", "err", err)
		return
	}
	if !shouldRun {
		slog.Info("higress appgroup already exists, skip upgrade shell")
		return
	}

	koDataPath, ok := os.LookupEnv("KO_DATA_PATH")
	if !ok {
		koDataPath = "./kodata"
	}

	scriptPath := filepath.Join(koDataPath, "shell", "upgradehigress.sh")
	stdout, stderr, err := helper.Runsh("bash", scriptPath, version)
	if stdout != "" {
		slog.Info("higress upgrade shell stdout", "output", stdout)
	}
	if stderr != "" {
		slog.Warn("higress upgrade shell stderr", "output", stderr)
	}
	if err != nil {
		slog.Error("run higress upgrade shell error", "script", scriptPath, "currentVersion", version, "targetVersion", version, "err", err)
		return
	}

	slog.Info("higress upgrade shell success", "script", scriptPath, "currentVersion", version, "targetVersion", version)
}

func shouldRunHigressUpgradeShell(sdk *k8s.Sdk) (bool, error) {
	groupApi, err := appgroup.NewAppGroupApi(sdk)
	if err != nil {
		return false, err
	}

	_, err = groupApi.GetAppGroup("default", higressGroupName)
	if err == nil {
		return false, nil
	}
	if apierrors.IsNotFound(err) {
		return true, nil
	}
	return false, err
}

func higressVersion(release *release.Release) string {
	if release == nil || release.Chart == nil || release.Chart.Metadata == nil {
		return ""
	}

	version := release.Chart.Metadata.AppVersion
	if version == "" {
		version = release.Chart.Metadata.Version
	}
	return strings.ReplaceAll(version, "v", "")
}
