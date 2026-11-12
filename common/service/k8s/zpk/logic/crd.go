package logic

import (
	"context"
	"errors"
	"fmt"
	"strings"

	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type UpgradeCRDResult struct {
	Names  []string `json:"names"`
	Digest string   `json:"digest"`
}

func requireUpgradeCRDApproval(result UpgradeCRDResult, approvedDigest string) error {
	if approvedDigest != "" && approvedDigest != result.Digest {
		return errors.New("Chart changed since CRD confirmation; check the upgrade again")
	}
	if len(result.Names) > 0 && approvedDigest == "" {
		return errors.New("CRD update requires confirmation")
	}
	return nil
}

func validateRootInstallOption(pack *types.ManifestPackage, options []types.InstallOption) error {
	for _, option := range options {
		if option.Identifie == pack.Manifest.Application.Identifie {
			return nil
		}
	}
	return errors.New("root install option is missing")
}

func upgradeChartCRDs(ctx context.Context, sdk *k8s.Sdk, pack *types.ManifestPackage, apps types.Package) (UpgradeCRDResult, map[string]*unstructured.Unstructured, error) {
	if !pack.Manifest.IsHelm() && pack.HelmUrl == "" {
		return UpgradeCRDResult{Names: []string{}}, nil, nil
	}
	config := pack.Manifest.Platform.Helm
	if pack.HelmUrl != "" {
		config.ChartName, config.Repository, config.Version = pack.HelmUrl, "", ""
	}
	chart, err := LocateChartByHelmZpk(config.Repository, config.ChartName, pack.ZipUrl, config.Version)
	if err != nil {
		return UpgradeCRDResult{}, nil, err
	}
	assignments := helmSetAssignments(apps.Root, "", []string{"HELM_ATOMIC", "DOMAIN_URL"}, false)
	for _, child := range apps.Children {
		prefix := strings.ReplaceAll(child.Manifest.Application.Identifie, "_", "-") + "."
		assignments = append(assignments, helmSetAssignments(child, prefix, []string{"HELM_ATOMIC"}, true)...)
	}
	digest, crds, err := upgradeCRDsFromChart(chart, assignments)
	if err != nil {
		return UpgradeCRDResult{}, nil, err
	}
	names, err := helmservice.ChangedCRDs(ctx, sdk, crds)
	return UpgradeCRDResult{Names: names, Digest: digest}, crds, err
}

func upgradeCRDsFromChart(ch *chart.Chart, assignments []string) (string, map[string]*unstructured.Unstructured, error) {
	digest, err := helmservice.ChartDigest(ch)
	if err != nil {
		return "", nil, err
	}
	if err := helmservice.ProcessChartDependencies(ch, assignments, nil); err != nil {
		return "", nil, err
	}
	crds, err := helmservice.ChartCRDs(ch)
	return digest, crds, err
}

// CheckUpgradeCRDs resolves the same remote artifact that installation uses.
func CheckUpgradeCRDs(ctx context.Context, sdk *k8s.Sdk, panelToken string, params InstallRequest) (UpgradeCRDResult, error) {
	params.IngressHost = normalizeInstallIngressHost(params.IngressHost)
	params.ReleaseName = strings.ReplaceAll(strings.ToLower(params.ReleaseName), "_", "-")
	group, err := appGroupForUpgrade(sdk, params.Namespace, params.ReleaseName)
	if err != nil {
		return UpgradeCRDResult{}, err
	}
	repo := NewRepo(params.RepoUrl, params.ThirdpartyCDToken, "")
	repo.SetDomain(installRequestDomain(params))
	repo.SetPanelToken(panelToken)
	repo.SetAppIdentify(params.ReleaseName)
	repo.SetReinstall(params.Reinstall)
	repo.SetUpgrade(true)
	repo.SetCurVersion(group.Spec.Version)
	pack, err := repo.Load()
	if err != nil {
		return UpgradeCRDResult{}, err
	}
	if err := validateRootInstallOption(pack, params.InstallOptions); err != nil {
		return UpgradeCRDResult{}, err
	}
	releaseName := params.ReleaseName
	if pack.Manifest.IsOnce() {
		releaseName = strings.ToLower(strings.ReplaceAll(pack.Manifest.Application.Identifie, "_", "-"))
	}
	if repo.IsConsole {
		preinstall, err := repo.PreInstall(params.ReleaseName)
		if err != nil {
			return UpgradeCRDResult{}, err
		}
		releaseName = preinstall.ReleaseName
		pack.ZipUrl = preinstall.ZipURL
	} else if params.IsTrandition {
		pack.ZipUrl = params.ZipUrl
	}
	namespace := params.Namespace
	if pack.Manifest.IsHelm() && pack.Manifest.GetHelmNamespce() != "" {
		namespace = pack.Manifest.GetHelmNamespce()
	}
	apps := types.NewPackage(pack, params.InstallOptions, releaseName, "crd-check", namespace,
		params.IngressHost, params.IngressSeletorName, params.IngressClassName)
	result, _, err := upgradeChartCRDs(ctx, sdk, pack, apps)
	return result, err
}

func appGroupForUpgrade(sdk *k8s.Sdk, namespace, name string) (*appv1.AppGroup, error) {
	group, err := appgroup.GetAppgroupUseSdk(name, namespace, sdk)
	if err != nil {
		return nil, fmt.Errorf("application %s is not installed: %w", name, err)
	}
	return group, nil
}
