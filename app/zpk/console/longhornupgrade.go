package console

import (
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	zpktypes "github.com/w7panel/w7panel/common/service/k8s/zpk/types"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/api/errors"
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

	groupApi, err := appgroup.NewAppGroupApi(sdk)
	if err != nil {
		slog.Error("new appgroup api error", "err", err)
		return
	}

	group := releaseToLonghornAppGroup(release)
	_, err = groupApi.GetAppGroup(group.Namespace, group.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			group.Namespace = "default"
			_, err = groupApi.CreateGroup(group.Namespace, group)
			if err != nil {
				slog.Error("create longhorn appgroup error", "err", err)
				return
			}
			slog.Info("create longhorn appgroup success", "namespace", group.Namespace, "name", group.Name, "version", group.Spec.Version)
			return
		}
		slog.Error("get longhorn appgroup error", "err", err)
		return
	}
	slog.Info("longhorn appgroup already exists, skip create", "namespace", group.Namespace, "name", group.Name)
}

func releaseToLonghornAppGroup(release *release.Release) *appv1.AppGroup {
	annotations := release.Chart.Metadata.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}

	identifie := "w7panel-longhorn"

	title := annotations[zpktypes.HELM_TITLE]
	if title == "" {
		title = release.Name
	}

	logo := annotations[zpktypes.HELM_LOGO]
	if logo == "" {
		logo = release.Chart.Metadata.Icon
	}

	version := release.Chart.Metadata.AppVersion
	if version == "" {
		version = release.Chart.Metadata.Version
	}
	version = strings.ReplaceAll(version, "v", "")
	group := appgroup.CreateAppGroup(release.Name, release.Namespace)
	group.Labels = map[string]string{
		"w7.cc/identifie": identifie,
	}
	group.Spec = appv1.AppGroupSpec{
		Identifie:   identifie,
		Type:        appv1.HELM,
		Version:     version,
		Title:       title,
		Logo:        logo,
		Suffix:      release.Name,
		ZpkUrl:      "https://zpk.w7.cc/zpk/respo/info/w7panel_longhorn",
		IsHelm:      true,
		Description: release.Chart.Metadata.Description,
		HelmConfig: appv1.HelmConfig{
			ChartName:  annotations[zpktypes.HELM_CHART_NAME],
			Repository: annotations[zpktypes.HELM_REPOSITORY_URL],
			Version:    version,
		},
	}
	group.Status = appv1.AppGroupStatus{
		Items:        []appv1.AppGroupItemStatus{},
		DeployItems:  []appv1.DeployItem{},
		DeployStatus: appv1.StatusDeployed,
		Ready:        true,
	}
	group.Annotations = map[string]string{
		zpktypes.HELM_DENY_DELETE: "true",
	}
	return group
}
