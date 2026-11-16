package logic

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/dnsprofile"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DependencyBinding struct {
	Namespace   string `json:"namespace,omitempty"`
	ReleaseName string `json:"releaseName"`
	Identifie   string `json:"identifie,omitempty"`
}

func appGroupDependencyReferences(bindings []DependencyBinding) []appgroup.DependencyReference {
	references := make([]appgroup.DependencyReference, 0, len(bindings))
	for _, binding := range bindings {
		references = append(references, appgroup.DependencyReference{
			Namespace: binding.Namespace,
			Name:      binding.ReleaseName,
			Identifie: binding.Identifie,
		})
	}
	return references
}

type InstallRequest struct {
	Namespace          string                `json:"namespace" binding:"required"`
	RepoUrl            string                `json:"repoUrl" binding:"required"`
	ReleaseName        string                `json:"releaseName" binding:"required"`
	InstallOptions     []types.InstallOption `json:"installOptions" binding:"required"`
	Dependencies       []DependencyBinding   `json:"dependencies,omitempty"`
	IngressHost        string                `json:"ingressHost"`        // 域名
	IngressSeletorName string                `json:"ingressSeletorName"` // 域名选择业务名称
	IngressClassName   string                `json:"ingressClass"`       // 域名选择业务名称
	IngressForceHttps  bool                  `json:"ingressForceHttps"`  // forceHttps
	DNSProfileID       string                `json:"dnsProfileId,omitempty"`
	AutoResolve        bool                  `json:"autoResolve,omitempty"`
	ThirdpartyCDToken  string                `json:"thirdpartyCDToken"` // 域名选择业务名称
	ClusterId          string                `json:"clusterId"`         // 集群ID
	IsTrandition       bool                  `json:"isTrandition"`      // 是否传统应用
	ZipUrl             string                `json:"zipUrl"`            // 代码包地址
	PanelUrl           string                `json:"panelUrl"`          // 安装时候的面板地址
	Reinstall          bool                  `json:"reinstall"`         // 强制覆盖旧安装绑定，仅允许非升级安装
	CRDApprovalDigest  string                `json:"crdApprovalDigest,omitempty"`
}
type InstallResult struct {
	ReleaseName     string `json:"releaseName"`
	InstallID       string `json:"installId"`
	Namespace       string `json:"namespace"`
	DNSResolveError string `json:"dnsResolveError,omitempty"`
}
type InstallExecution struct {
	SDK          *k8s.Sdk
	Identity     *k8s.K8sToken
	PanelToken   string
	InstallID    string
	CallbackHost string
	install      func(types.Package, string, string) error
}

// ExecuteInstall shares the HTTP installation workflow without HTTP authentication.
func ExecuteInstall(params InstallRequest, execution InstallExecution) (InstallResult, error) {
	if execution.SDK == nil {
		return InstallResult{}, errors.New("installation SDK is required")
	}
	params.IngressHost = normalizeInstallIngressHost(params.IngressHost)

	repoUrl := params.RepoUrl
	if repoUrl == "" {
		return InstallResult{}, errors.New("repo url is empty")
	}
	token := execution.PanelToken
	k8sToken := execution.Identity
	client := execution.SDK
	params.ReleaseName = strings.ReplaceAll(strings.ToLower(params.ReleaseName), "_", "-")
	// helmApi := helmservice.NewHelm(client)
	appgroupObj, err := appgroup.GetAppgroupUseSdk(params.ReleaseName, params.Namespace, client)
	if err != nil && !apierrors.IsNotFound(err) {
		return InstallResult{}, err
	}
	isUpgrade := err == nil && appgroupObj != nil

	repo := NewRepo(repoUrl, params.ThirdpartyCDToken, "")
	repo.SetDomain(installRequestDomain(params))
	repo.SetAppIdentify(params.ReleaseName)
	repo.SetReinstall(params.Reinstall)
	if err == nil {
		repo.SetUpgrade(true)
		if appgroupObj != nil {
			repo.SetCurVersion(appgroupObj.Spec.Version)
		}
	}
	repo.SetPanelToken(token)
	// os.Setenv("KUBERNETES_SERVICE_HOST", "172.16.1.13")
	// os.Setenv("KUBERNETES_SERVICE_PORT", "6443")
	namespace := params.Namespace
	mPackage, err := repo.Load()
	if err != nil {
		return InstallResult{}, err
	}
	if err := validateRootInstallOption(mPackage, params.InstallOptions); err != nil {
		return InstallResult{}, err
	}
	if mPackage.Manifest.IsHelm() {
		helmNs := mPackage.Manifest.GetHelmNamespce()
		if helmNs != "" {
			namespace = helmNs
		}
	}

	if err := prepareInstallDomainGroups(mPackage, &params); err != nil {
		return InstallResult{}, err
	}

	_, errns := client.CreateNamespace(namespace)
	if errns != nil {
		slog.Warn("create namespace error", "err", err)
		// return
	}

	releaseName := strings.ToLower(params.ReleaseName)
	if mPackage.Manifest.IsOnce() {
		releaseName = strings.ToLower(strings.ReplaceAll(mPackage.Manifest.Application.Identifie, "_", "-"))
	}
	appSecret := &console.AppSecret{}
	if repo.IsConsole {
		// if (params.ClusterId == "") {
		// 	params.ClusterId = config.MainW7Config.ClusterId
		// }
		preinstall, err := repo.PreInstall(params.ReleaseName)
		if err != nil {
			return InstallResult{}, err
		}
		releaseName = preinstall.ReleaseName
		zipUrl := preinstall.ZipURL
		mPackage.ZipUrl = zipUrl

		// cdClient := console.NewConsoleCdClient(params.ThirdpartyCDToken)
		// appSecret, err = cdClient.CreateSite(params.IngressHost, releaseName)
		// if err != nil {
		// 	slog.Warn("create site error may not need secret", "err", err)
		// }
	}
	if params.PanelUrl != "" {
		mPackage.PanelUrl = params.PanelUrl
	}
	if params.IsTrandition {
		mPackage.ZipUrl = params.ZipUrl

	}
	//随机k8s deployment name
	installId := execution.InstallID
	if installId == "" {
		installId = helper.RandomString(5)
	}
	// releaseName = params.ReleaseName
	primaryDomain := primaryInstallDomain(mPackage.Manifest.Application.Identifie, params)
	packageApps := types.NewPackage(mPackage, params.InstallOptions, releaseName, installId, namespace,
		primaryDomain.Host, params.IngressSeletorName, params.IngressClassName)
	var upgradeCRDs map[string]*unstructured.Unstructured
	if isUpgrade {
		check, crds, err := upgradeChartCRDs(client.Ctx, client, mPackage, packageApps)
		if err != nil {
			return InstallResult{}, err
		}
		if err := requireUpgradeCRDApproval(check, params.CRDApprovalDigest); err != nil {
			return InstallResult{}, err
		}
		packageApps.Root.ChartApprovalDigest = check.Digest
		// ZPK Chart URLs can be single-use; the CRD check has consumed this one.
		if mPackage.HelmUrl != "" || (mPackage.Manifest.IsHelm() && mPackage.Manifest.Platform.Helm.Repository == "" && mPackage.Manifest.Platform.Helm.ChartName == "") {
			zipURL, helmURL, err := repo.FreshChartURLs(params.ReleaseName)
			if err != nil {
				return InstallResult{}, err
			}
			if mPackage.HelmUrl != "" {
				if helmURL == "" {
					return InstallResult{}, errors.New("fresh Helm Chart download URL is missing")
				}
				mPackage.HelmUrl = helmURL
			} else {
				if zipURL == "" {
					return InstallResult{}, errors.New("fresh Helm Chart zip URL is missing")
				}
				mPackage.ZipUrl = zipURL
			}
		}
		if len(check.Names) > 0 {
			upgradeCRDs = crds
		}
	}
	dependencies, err := appgroup.ResolveDependencies(
		client,
		namespace,
		appGroupDependencyReferences(params.Dependencies),
	)
	if err != nil {
		return InstallResult{}, err
	}
	packageApps.Root.AppGroupDependencies = dependencies
	packageApps.ForceHttps(primaryDomain.AutoSSL)

	for _, app := range append([]*types.PackageApp{packageApps.Root}, packageApps.Children...) {
		if app.Replicas == 0 {
			continue
		}
		for _, binding := range app.DomainGroups {
			if binding.Host == "" || !binding.AutoSSL || !strings.HasPrefix(binding.Host, "*.") {
				continue
			}
			if k8sToken == nil {
				return InstallResult{}, errors.New("泛域名自动 HTTPS 需要用户身份")
			}
			owner, err := k8sToken.GetUserName()
			if err != nil {
				return InstallResult{}, err
			}
			store := dnsprofile.Store{SDK: client}
			grant, err := store.Grant(context.Background(), owner, binding.DNSProfileID, namespace, dnsprofile.HelmGrantScope(releaseName), binding.Host)
			if err != nil {
				return InstallResult{}, err
			}
			packageApps.Root.IngressDNSGrants = append(packageApps.Root.IngressDNSGrants, dnsprofile.HelmGrant{
				Host: binding.Host, ProfileID: binding.DNSProfileID, Signature: grant[dnsprofile.GrantAnnotation],
			})
		}
	}

	realToken := ""
	config, err := client.ToRESTConfig()
	if err != nil {
		slog.Warn("client config err", "err", err)
	}
	if config != nil {
		realToken = config.BearerToken
	}
	sa := client.GetServiceAccountName()
	packageApps.Root.ServiceAccountName = sa
	packageApps.Root.K8sToken = k8sToken
	packageApps.Root.RealToken = realToken
	for _, child := range packageApps.Children {
		child.ServiceAccountName = sa
		child.K8sToken = k8sToken
		child.RealToken = realToken

	}

	// 微擎有安装脚本需要预先获取appid和secret
	if appSecret != nil && appSecret.AppId != "" && appSecret.AppSecret != "" {
		appId := types.Env{}
		appId.Name = "APP_ID"
		appId.Value = appSecret.AppId
		secret := types.Env{}
		secret.Name = "APP_SECRET"
		secret.Value = appSecret.AppSecret
		packageApps.Root.Manifest.Platform.Container.Env = append(packageApps.Root.Manifest.Platform.Container.Env, appId, secret)
	}
	if err != nil {
		return InstallResult{}, err
	}

	if execution.CallbackHost != "" {
		packageApps.Root.InstallOption.BuildImageSuccessUrl = "http://" + execution.CallbackHost + "/panel-api/v1/zpk/build-image-success?namespace=" + params.Namespace + "&releaseName=" + releaseName + "&domainHost=" + primaryDomain.Host + "&deploymentName=" + packageApps.Root.GetName() + "&thirdpartyCDToken=" + params.ThirdpartyCDToken + "&api-token=" + token
	}
	packageApps.Root.ThirdpartyCDToken = params.ThirdpartyCDToken

	// 父节点发布名不为空 且 需要父节点发布名 且 有父节点 则设置父节点
	// 20250321
	// PackageApp label annnoations 添加 ["w7.cc/parent"]
	if packageApps.Root.ManifestPackage.RequireParentReleaseName &&
		packageApps.Root.ManifestPackage.Parent != nil &&
		packageApps.Root.ParentReleaseName != "" {
		parent := types.NewPackageApp(packageApps.Root.ManifestPackage.Parent, &types.InstallOption{ReleaseName: packageApps.Root.ParentReleaseName})
		packageApps.Root.Parent = parent
	}
	if len(upgradeCRDs) > 0 {
		if err := helmservice.ApplyCRDs(client.Ctx, client, upgradeCRDs); err != nil {
			return InstallResult{}, err
		}
	}
	helper.Set(releaseName, installId, time.Minute*30) //site.go 注册站点 installId 要匹配
	if execution.install != nil {
		err = execution.install(packageApps, releaseName, namespace)
	} else {
		install := NewInstall(client, packageApps)
		if install == nil {
			return InstallResult{}, errors.New("initialize installer failed")
		}
		err = install.InstallOrUpgrade(releaseName, namespace)
	}
	if err != nil {
		// panic(err)
		return InstallResult{}, err
	}
	result := InstallResult{ReleaseName: releaseName, InstallID: installId, Namespace: namespace}
	if params.AutoResolve && strings.HasPrefix(primaryDomain.Host, "*.") {
		if primaryDomain.DNSProfileID == "" || k8sToken == nil {
			result.DNSResolveError = "缺少已验证的 DNS 配置"
		} else if owner, ownerErr := k8sToken.GetUserName(); ownerErr != nil {
			result.DNSResolveError = ownerErr.Error()
		} else if resolveErr := (dnsprofile.Store{SDK: client}).EnsureRecord(context.Background(), owner, primaryDomain.DNSProfileID, primaryDomain.Host); resolveErr != nil {
			result.DNSResolveError = resolveErr.Error()
		}
	}
	return result, nil
}

func normalizeInstallIngressHost(host string) string {
	host = strings.ToLower(host)
	host = strings.ReplaceAll(host, "https://", "")
	host = strings.ReplaceAll(host, "http://", "")
	return strings.ReplaceAll(host, "/", "")
}

func installRequestDomain(params InstallRequest) string {
	if params.IngressHost != "" {
		return params.IngressHost
	}

	if len(params.InstallOptions) > 0 && len(params.InstallOptions[0].DomainGroups) > 0 {
		for _, binding := range params.InstallOptions[0].DomainGroups {
			if host := normalizeInstallDomainHost(binding.Host); host != "" {
				return host
			}
		}
		return ""
	}
	for _, option := range params.InstallOptions {
		for _, env := range option.EnvKv {
			name := strings.ToUpper(strings.TrimSpace(env.Name))
			if name != "DOMAIN_URL" && name != "DOMAIN_SSL_URL" {
				continue
			}
			if domain := normalizeInstallDomainHost(env.Value); domain != "" {
				return domain
			}
		}
	}
	return ""
}

func normalizeInstallDomainHost(value string) string {
	if strings.Contains(strings.ToUpper(value), "%DOMAIN_") {
		return ""
	}
	parsed, ok := helper.ParseDomainURL(value, "http://")
	if !ok {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

// primaryInstallDomain adapts the primary group's data for legacy callers without
// overwriting request fields or the configuration of any other domain group.
func primaryInstallDomain(rootIdentifie string, params InstallRequest) types.DomainBinding {
	for _, option := range params.InstallOptions {
		if option.Identifie != rootIdentifie || len(option.DomainGroups) == 0 {
			continue
		}
		for _, binding := range option.DomainGroups {
			if binding.Host != "" {
				return binding
			}
		}
		return types.DomainBinding{}
	}
	return types.DomainBinding{Param: types.DefaultDomainGroup, Host: params.IngressHost, AutoSSL: params.IngressForceHttps, DNSProfileID: params.DNSProfileID}
}

// Validate grouped input before creating namespaces or installation resources.
func prepareInstallDomainGroups(pack *types.ManifestPackage, params *InstallRequest) error {
	rootIndex := -1
	seen := map[string]bool{}
	for i, option := range params.InstallOptions {
		manifest := pack.GetChildren(option.Identifie)
		if manifest == nil {
			return errors.New("安装选项包含未知应用")
		}
		identifie := manifest.Manifest.Application.Identifie
		if seen[identifie] {
			return errors.New("安装应用不能重复")
		}
		seen[identifie] = true
		if manifest == pack {
			rootIndex = i
		}
	}
	if rootIndex < 0 {
		return errors.New("缺少主应用安装选项")
	}

	root := &params.InstallOptions[rootIndex]
	rootRequest := *params
	rootRequest.InstallOptions = []types.InstallOption{*root}
	authorizedDomain := normalizeInstallDomainHost(installRequestDomain(rootRequest))
	if err := types.PrepareDomainGroups(pack.Manifest, root, authorizedDomain, params.IngressForceHttps, params.DNSProfileID); err != nil {
		return err
	}
	primary := primaryInstallDomain(pack.Manifest.Application.Identifie, *params)
	if len(root.DomainGroups) > 0 && primary.Host != authorizedDomain {
		return errors.New("授权域名与首个已填写的域名分组不一致")
	}

	for i := range params.InstallOptions {
		if i == rootIndex {
			continue
		}
		option := &params.InstallOptions[i]
		manifest := pack.GetChildren(option.Identifie)
		// Normalize old children once at the input boundary. Explicit bindings
		// and declared groups keep their independent configuration.
		namedGroups := false
		for _, group := range manifest.Manifest.Platform.Ingress {
			if group.DomainParam != "" && len(group.Routes) > 0 {
				namedGroups = true
				break
			}
		}
		if !namedGroups && manifest.Manifest.RequireDomain() && len(option.DomainGroups) == 0 {
			binding := primary
			binding.Param = types.DefaultDomainGroup
			option.DomainGroups = []types.DomainBinding{binding}
		}
		if err := types.PrepareDomainGroups(manifest.Manifest, option, "", false, ""); err != nil {
			return err
		}
	}
	return nil
}
