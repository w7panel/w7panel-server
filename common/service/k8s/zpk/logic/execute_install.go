package logic

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	bi "github.com/w7panel/w7panel/common/service/k8s/buildimage"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic/types"
)

type InstallRequest struct {
	Namespace          string                `json:"namespace" binding:"required"`
	RepoUrl            string                `json:"repoUrl" binding:"required"`
	ReleaseName        string                `json:"releaseName" binding:"required"`
	InstallOptions     []types.InstallOption `json:"installOptions" binding:"required"`
	IngressHost        string                `json:"ingressHost"`        // 域名
	IngressSeletorName string                `json:"ingressSeletorName"` // 域名选择业务名称
	IngressClassName   string                `json:"ingressClass"`       // 域名选择业务名称
	IngressForceHttps  bool                  `json:"ingressForceHttps"`  // forceHttps
	ThirdpartyCDToken  string                `json:"thirdpartyCDToken"`  // 域名选择业务名称
	ClusterId          string                `json:"clusterId"`          // 集群ID
	IsTrandition       bool                  `json:"isTrandition"`       // 是否传统应用
	ZipUrl             string                `json:"zipUrl"`             // 代码包地址
	PanelUrl           string                `json:"panelUrl"`           // 安装时候的面板地址
	Reinstall          bool                  `json:"reinstall"`          // 强制覆盖旧安装绑定，仅允许非升级安装
}
type InstallResult struct {
	ReleaseName string `json:"releaseName"`
	InstallID   string `json:"installId"`
	Namespace   string `json:"namespace"`
}
type InstallExecution struct {
	SDK          *k8s.Sdk
	Identity     *k8s.K8sToken
	PanelToken   string
	IsChild      bool
	InstallID    string
	CallbackHost string
	install      func(types.Package, string, string) error
}

// ExecuteInstall shares the HTTP installation workflow without HTTP authentication.
func ExecuteInstall(params InstallRequest, execution InstallExecution) (InstallResult, error) {
	if execution.SDK == nil {
		return InstallResult{}, errors.New("installation SDK is required")
	}
	params.IngressHost = strings.ToLower(params.IngressHost)
	params.IngressHost = strings.ReplaceAll(params.IngressHost, "https://", "")
	params.IngressHost = strings.ReplaceAll(params.IngressHost, "http://", "")
	params.IngressHost = strings.ReplaceAll(params.IngressHost, "/", "")

	repoUrl := params.RepoUrl
	if repoUrl == "" {
		return InstallResult{}, errors.New("repo url is empty")
	}
	token := execution.PanelToken
	k8sToken := execution.Identity
	client := execution.SDK
	params.ReleaseName = strings.ReplaceAll(strings.ToLower(params.ReleaseName), "_", "-")
	// helmApi := k8s.NewHelm(client)
	appgroupObj, err := appgroup.GetAppgroupUseSdk(params.ReleaseName, client.GetNamespace(), client)

	repo := NewRepo(repoUrl, params.ThirdpartyCDToken, "")
	repo.SetDomain(params.IngressHost)
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
	if mPackage.Manifest.IsHelm() {
		helmNs := mPackage.Manifest.GetHelmNamespce()
		if helmNs != "" {
			namespace = helmNs
		}
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
		preinstall, err := repo.PreInstall()
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

	packageApps := types.NewPackage(mPackage, params.InstallOptions, releaseName, installId, namespace,
		params.IngressHost, params.IngressSeletorName, params.IngressClassName)
	packageApps.ForceHttps(params.IngressForceHttps)
	// packageApps.Root.K3kMode = k8sToken.K3kMode()
	isChild := execution.IsChild

	realToken := ""
	config, err := client.ToRESTConfig()
	if err != nil {
		slog.Warn("client config err", "err", err)
	}
	if config != nil {
		realToken = config.BearerToken
	}
	if execution.IsChild {
		registryHost, err := bi.PanelRegistryServerHostUseSdk(client)
		if err != nil {
			slog.Warn("get registry host err", "err", err)
		} else {
			packageApps.Root.PanelRegistryServerHost = registryHost
		}
	}

	sa := client.GetServiceAccountName()
	packageApps.Root.ServiceAccountName = sa
	packageApps.Root.K8sToken = k8sToken
	packageApps.Root.IsChild = isChild
	packageApps.Root.RealToken = realToken
	for _, child := range packageApps.Children {
		child.ServiceAccountName = sa
		if execution.IsChild {
			child.IngressClassName = packageApps.Root.IngressClassName
		}
		child.K8sToken = k8sToken
		child.IsChild = execution.IsChild
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
		packageApps.Root.InstallOption.BuildImageSuccessUrl = "http://" + execution.CallbackHost + "/panel-api/v1/zpk/build-image-success?namespace=" + params.Namespace + "&releaseName=" + releaseName + "&domainHost=" + params.IngressHost + "&deploymentName=" + packageApps.Root.GetName() + "&thirdpartyCDToken=" + params.ThirdpartyCDToken + "&api-token=" + token
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
	return InstallResult{ReleaseName: releaseName, InstallID: installId, Namespace: namespace}, nil
}
