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
	artifactv1 "github.com/w7panel/w7panel/k8s/pkg/apis/artifactinstallation/v1alpha1"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type artifactInstaller interface {
	Lookup(context.Context, *artifactv1.ArtifactInstallation) (*installedArtifact, error)
	Install(context.Context, *artifactv1.ArtifactInstallation) error
	Uninstall(context.Context, *artifactv1.ArtifactInstallation) error
}

type installedArtifactState string

const (
	installedArtifactInstalling installedArtifactState = "Installing"
	installedArtifactReady      installedArtifactState = "Ready"
	installedArtifactFailed     installedArtifactState = "Failed"
	installedArtifactDeleting   installedArtifactState = "Deleting"
)

var (
	errArtifactAlreadyExists = errors.New("artifact already exists")
	errArtifactDeleting      = errors.New("artifact is deleting")
)

type installedArtifact struct {
	Name      string
	Namespace string
	Identifie string
	Version   string
	State     installedArtifactState
	Owned     bool
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

func (i *zpkArtifactInstaller) load(ctx context.Context, reference bootstrapv1.ArtifactReference) (*zpktypes.ManifestPackage, error) {
	if strings.HasPrefix(reference.Source, "oci://") {
		return nil, errors.New("当前 ZPK 加载器尚不支持 OCI BootstrapProfile source")
	}
	repo := logic.NewRepo(reference.Source, "", "")
	repo.SetPanelToken(i.panelToken)
	if reference.Version != "" {
		repo.SetTargetVersion(reference.Version)
	}
	pack, err := repo.LoadContext(ctx)
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

func (i *zpkArtifactInstaller) Lookup(ctx context.Context, installation *artifactv1.ArtifactInstallation) (*installedArtifact, error) {
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
	state := appGroupArtifactState(group)
	return &installedArtifact{
		Name: group.Name, Namespace: group.Namespace,
		Identifie: group.Spec.Identifie, Version: group.Spec.Version,
		State: state,
		Owned: isArtifactOwner(group.Annotations, installation),
	}, nil
}

func appGroupArtifactState(group *appgroupv1.AppGroup) installedArtifactState {
	switch {
	case !group.DeletionTimestamp.IsZero():
		return installedArtifactDeleting
	case group.Status.DeployStatus == appgroupv1.StatusFailed || group.Status.ComputeDeployIsFailed():
		return installedArtifactFailed
	case group.Status.Ready && group.Status.DeployStatus == appgroupv1.StatusDeployed:
		return installedArtifactReady
	default:
		return installedArtifactInstalling
	}
}

func (i *zpkArtifactInstaller) Install(ctx context.Context, installation *artifactv1.ArtifactInstallation) error {
	if effectiveArtifactType(installation.Spec.Artifact.Type) != bootstrapv1.ArtifactTypeZPK {
		return fmt.Errorf("制品类型 %q 当前不支持", installation.Spec.Artifact.Type)
	}
	reference := installation.Spec.Artifact
	pack, err := i.load(ctx, reference)
	if err != nil {
		return err
	}

	if _, err := i.sdk.ClientSet.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: installation.Spec.Target.Namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("创建命名空间 %q: %w", installation.Spec.Target.Namespace, err)
	}
	options := make([]zpktypes.InstallOption, 0, len(pack.Children)+1)
	current, err := i.Lookup(ctx, installation)
	if err != nil {
		return err
	}
	if current != nil {
		if current.State == installedArtifactDeleting {
			return errArtifactDeleting
		}
		return errArtifactAlreadyExists
	}
	options = append(options, zpktypes.InstallOption{
		Identifie:   pack.Manifest.Application.Identifie,
		Replicas:    1,
		HelmValues:  cloneStringMap(installation.Spec.InstallOptions.HelmValues),
		Annotations: artifactInstallAnnotations(installation),
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
	scopedSDK := *i.sdk
	scopedSDK.Ctx = ctx
	installer := logic.NewInstall(&scopedSDK, packages)
	if installer == nil {
		return errors.New("初始化 ZPK 安装器失败")
	}
	if err := installer.Install(installation.Spec.Target.ReleaseName, installation.Spec.Target.Namespace); err != nil {
		return fmt.Errorf("执行 ZPK Install: %w", err)
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

func (i *zpkArtifactInstaller) Uninstall(ctx context.Context, installation *artifactv1.ArtifactInstallation) error {
	current, err := i.Lookup(ctx, installation)
	if err != nil {
		return err
	}
	if current == nil || current.State == installedArtifactDeleting || !current.Owned {
		return nil
	}
	sigClient, err := i.sdk.ToSigClient()
	if err != nil {
		return fmt.Errorf("创建 AppGroup 客户端: %w", err)
	}
	group := &appgroupv1.AppGroup{}
	group.Name = current.Name
	group.Namespace = current.Namespace
	if err := sigClient.Delete(ctx, group); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("删除 AppGroup %s/%s: %w", current.Namespace, current.Name, err)
	}
	return nil
}

func artifactOwner(installation *artifactv1.ArtifactInstallation) string {
	return installation.Spec.ProfileRef.UID + "/" + installation.Spec.Artifact.Name
}

func artifactInstallAnnotations(installation *artifactv1.ArtifactInstallation) map[string]string {
	annotations := cloneStringMap(installation.Spec.InstallOptions.Annotations)
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}
	annotations[bootstrapv1.AnnotationArtifactOwner] = artifactOwner(installation)
	return annotations
}

func isArtifactOwner(annotations map[string]string, installation *artifactv1.ArtifactInstallation) bool {
	return annotations[bootstrapv1.AnnotationArtifactOwner] == artifactOwner(installation)
}

func normalizeIdentifie(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "_", "-")
}
