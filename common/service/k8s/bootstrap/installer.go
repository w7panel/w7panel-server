package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/app/zpk/logic"
	zpktypes "github.com/w7panel/w7panel/app/zpk/logic/types"
	"github.com/w7panel/w7panel/common/service/k8s"
	appgroupv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type artifactInstaller interface {
	Lookup(context.Context, *bootstrapv1.ArtifactInstallation) (*installedArtifact, error)
	InstallOrUpgrade(context.Context, *bootstrapv1.ArtifactInstallation) error
	Uninstall(context.Context, string, string) error
}

type installedArtifact struct {
	Name      string
	Namespace string
	Identifie string
	Version   string
}

type zpkArtifactInstaller struct {
	sdk        *k8s.Sdk
	panelToken string
}

func newZPKArtifactInstaller(sdk *k8s.Sdk) (*zpkArtifactInstaller, error) {
	config, err := sdk.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("读取 Kubernetes REST 配置: %w", err)
	}
	return &zpkArtifactInstaller{sdk: sdk, panelToken: config.BearerToken}, nil
}

func (i *zpkArtifactInstaller) load(reference bootstrapv1.ArtifactReference) (*zpktypes.ManifestPackage, error) {
	if strings.HasPrefix(reference.Source, "oci://") {
		return nil, errors.New("当前 ZPK 加载器尚不支持 OCI BootstrapProfile source")
	}
	repo := logic.NewRepo(reference.Source, "", "")
	repo.SetPanelToken(i.panelToken)
	if reference.Version != "" {
		repo.SetCurVersion(reference.Version)
	}
	pack, err := repo.Load()
	if err != nil {
		return nil, fmt.Errorf("加载制品 %q: %w", reference.Source, err)
	}
	actualIdentifie := normalizeIdentifie(pack.Manifest.Application.Identifie)
	if actualIdentifie != normalizeIdentifie(reference.Identifie) {
		return nil, fmt.Errorf("制品 identifie 不匹配: 期望 %q，实际 %q", reference.Identifie, pack.Manifest.Application.Identifie)
	}
	if reference.Version != "" && compareVersions(pack.Version.Name, reference.Version) != 0 {
		return nil, fmt.Errorf("制品库未返回指定版本: 期望 %q，实际 %q", reference.Version, pack.Version.Name)
	}
	return pack, nil
}

func (i *zpkArtifactInstaller) Lookup(ctx context.Context, installation *bootstrapv1.ArtifactInstallation) (*installedArtifact, error) {
	sigClient, err := i.sdk.ToSigClient()
	if err != nil {
		return nil, fmt.Errorf("创建 AppGroup 客户端: %w", err)
	}
	group := &appgroupv1.AppGroup{}
	key := client.ObjectKey{Name: installation.Spec.Target.ReleaseName, Namespace: installation.Spec.Target.Namespace}
	if err := sigClient.Get(ctx, key, group); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("查询 AppGroup %s/%s: %w", key.Namespace, key.Name, err)
	}
	if !group.DeletionTimestamp.IsZero() {
		return nil, nil
	}
	return &installedArtifact{
		Name: group.Name, Namespace: group.Namespace,
		Identifie: group.Spec.Identifie, Version: group.Spec.Version,
	}, nil
}

func (i *zpkArtifactInstaller) InstallOrUpgrade(ctx context.Context, installation *bootstrapv1.ArtifactInstallation) error {
	if effectiveArtifactType(installation.Spec.Artifact.Type) != bootstrapv1.ArtifactTypeZPK {
		return fmt.Errorf("制品类型 %q 当前不支持", installation.Spec.Artifact.Type)
	}
	reference := installation.Spec.Artifact
	pack, err := i.load(reference)
	if err != nil {
		return err
	}

	if _, err := i.sdk.CreateNamespace(installation.Spec.Target.Namespace); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建命名空间 %q: %w", installation.Spec.Target.Namespace, err)
	}
	options := make([]zpktypes.InstallOption, 0, len(pack.Children)+1)
	helmValues := installation.Spec.InstallOptions.HelmValues
	sigClient, err := i.sdk.ToSigClient()
	if err != nil {
		return fmt.Errorf("创建 AppGroup 客户端: %w", err)
	}
	existingGroup := &appgroupv1.AppGroup{}
	if err := sigClient.Get(ctx, client.ObjectKey{Name: installation.Spec.Target.ReleaseName, Namespace: installation.Spec.Target.Namespace}, existingGroup); err == nil {
		// Profile 中的 Helm 参数是首次安装默认值，升级不覆盖用户当前 Values。
		helmValues = nil
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("查询 AppGroup %s/%s: %w", installation.Spec.Target.Namespace, installation.Spec.Target.ReleaseName, err)
	}
	options = append(options, zpktypes.InstallOption{
		Identifie:  pack.Manifest.Application.Identifie,
		Replicas:   1,
		HelmValues: cloneStringMap(helmValues),
	})
	for name, child := range pack.Children {
		replicas := int32(0)
		if child.RequireInstall {
			replicas = 1
		}
		options = append(options, zpktypes.InstallOption{Identifie: name, Replicas: replicas})
	}

	installID := installation.Status.OperationID
	if len(installID) > 12 {
		installID = installID[:12]
	}
	packages := zpktypes.NewPackage(pack, options, installation.Spec.Target.ReleaseName, installID,
		installation.Spec.Target.Namespace, "", "", "")
	if packages.Root == nil {
		return errors.New("制品未生成根安装项")
	}
	packages.Root.ServiceAccountName = i.sdk.GetServiceAccountName()
	packages.Root.RealToken = i.panelToken
	packages.Root.K8sToken = k8s.NewK8sToken(i.panelToken)
	for _, child := range packages.Children {
		child.ServiceAccountName = packages.Root.ServiceAccountName
		child.RealToken = i.panelToken
		child.K8sToken = packages.Root.K8sToken
	}
	installer := logic.NewInstall(i.sdk, packages)
	if installer == nil {
		return errors.New("初始化 ZPK 安装器失败")
	}
	if err := installer.InstallOrUpgrade(installation.Spec.Target.ReleaseName, installation.Spec.Target.Namespace); err != nil {
		return fmt.Errorf("执行 ZPK InstallOrUpgrade: %w", err)
	}
	return nil
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (i *zpkArtifactInstaller) Uninstall(ctx context.Context, releaseName, namespace string) error {
	sigClient, err := i.sdk.ToSigClient()
	if err != nil {
		return fmt.Errorf("创建 AppGroup 客户端: %w", err)
	}
	group := &appgroupv1.AppGroup{}
	group.Name = releaseName
	group.Namespace = namespace
	if err := sigClient.Delete(ctx, group); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 AppGroup %s/%s: %w", namespace, releaseName, err)
	}
	return nil
}

func normalizeIdentifie(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "_", "-")
}
