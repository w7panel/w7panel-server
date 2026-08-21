package logic

import (
	"log/slog"
	"strings"

	"github.com/w7panel/w7panel/app/zpk/logic/types"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	convert "github.com/w7panel/w7panel/common/service/k8s/zpk"
	helm "github.com/w7panel/w7panel/common/service/k8s/zpk"
	zpktypes "github.com/w7panel/w7panel/common/service/k8s/zpk/types"
	"github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Install struct {
	sdk      *k8s.Sdk
	pk       types.Package
	groupApi *appgroup.AppGroupApi
}

func NewInstall(sdk *k8s.Sdk, pk types.Package) *Install {
	groupApi, err := appgroup.NewAppGroupApi(sdk)
	if err != nil {
		return nil
	}
	return &Install{sdk: sdk, pk: pk, groupApi: groupApi}
}

const appGroupParentLabel = "w7.cc/parent"

func (z *Install) parentGroupName() string {
	if z.pk.Root == nil || z.pk.Root.Manifest.Application.Annotation == nil {
		return ""
	}
	return strings.TrimSpace(z.pk.Root.Manifest.Application.Annotation[appGroupParentLabel])
}

func (z *Install) ensureParentGroup(group *v1alpha1.AppGroup, parentName string) error {
	parentName = strings.ToLower(strings.TrimSpace(parentName))
	if parentName == "" || group == nil || parentName == group.Name {
		return nil
	}
	if existing, err := z.groupApi.GetAppGroup(group.Namespace, parentName); err == nil {
		// Normalize parents created by an earlier version that still looked like
		// Helm AppGroups. Do not modify a real, user-created AppGroup with the
		// same name unless it carries our synthetic-parent marker.
		if existing.Annotations["w7.cc/parent-root"] == "true" && existing.Spec.IsHelm {
			existing.Spec.Type = "custom"
			existing.Spec.IsHelm = false
			existing.Spec.HelmConfig = v1alpha1.HelmConfig{}
			_, err = z.groupApi.UpdateAppGroup(group.Namespace, existing)
			return err
		}
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}
	parent := group.DeepCopy()
	parent.Name = parentName
	parent.ResourceVersion = ""
	parent.UID = ""
	parent.CreationTimestamp = metav1.Time{}
	parent.Labels = map[string]string{}
	for key, value := range group.Labels {
		parent.Labels[key] = value
	}
	delete(parent.Labels, appGroupParentLabel)
	parent.Labels["w7.cc/group-name"] = parentName
	parent.Labels["w7.cc/release-name"] = parentName
	// The parent is only an aggregation object. It must not be treated as a
	// real Helm release by upgrade/install flows.
	parent.Spec.Type = "custom"
	parent.Spec.IsHelm = false
	parent.Spec.HelmConfig = v1alpha1.HelmConfig{}
	parent.Status = v1alpha1.AppGroupStatus{DeployItems: []v1alpha1.DeployItem{}, Items: []v1alpha1.AppGroupItemStatus{}}
	if parent.Annotations == nil {
		parent.Annotations = map[string]string{}
	}
	// The synthetic root must not point to itself (or inherit the child's
	// parent annotation); it is the grouping target for all instances.
	delete(parent.Annotations, appGroupParentLabel)
	parent.Annotations["w7.cc/parent-root"] = "true"
	_, err := z.groupApi.CreateGroup(group.Namespace, parent)
	return err
}

func (z *Install) applyParentGroup(group *v1alpha1.AppGroup) error {
	parentName := z.parentGroupName()
	if parentName == "" || group == nil {
		return nil
	}
	if group.Labels == nil {
		group.Labels = map[string]string{}
	}
	group.Labels[appGroupParentLabel] = parentName
	return z.ensureParentGroup(group, parentName)
}

func (z *Install) InstallOrUpgrade(name, namespace string) error {
	_, err := z.Get(name, namespace)
	if err != nil {
		return z.Install(name, namespace)
	} else {
		return z.Upgrade(name, namespace)
	}
}

func (z *Install) createHelmJob(myPack *types.PackageApp, shellType types.ShellType, children []*types.PackageApp) (v1alpha1.DeployItem, []*batchv1.Job, error) {
	jobs := []*batchv1.Job{}
	job := toHelmInstallJob(myPack, children)
	jobs = append(jobs, job)
	shell := myPack.GetShellByType(string(shellType))
	var shellJob *batchv1.Job
	if shell != nil {
		shellJob = convert.ToShellJob2(myPack, myPack, string(shellType))
		jobs = append(jobs, shellJob)
	}
	info := v1alpha1.ResourceInfo{
		Name:         job.Name,
		Namespace:    myPack.GetNamespace(),
		Kind:         "Job",
		ApiVersion:   "batch/v1",
		DeployStatus: v1alpha1.StatusDeploying,
		DeployTitle:  "helm安装",
	}

	installResult := v1alpha1.DeployItem{
		Identifie:    myPack.GetIdentifie(),
		Title:        myPack.GetTitle(),
		ResourceList: []v1alpha1.ResourceInfo{info},
		DeployStatus: v1alpha1.StatusDeploying,
	}
	if shellJob != nil {
		// 新版制品库 已经自带了helm job
		// shellInfo := v1alpha1.ResourceInfo{
		// 	Name:         shellJob.Name,
		// 	Namespace:    myPack.GetNamespace(),
		// 	Kind:         "Job",
		// 	ApiVersion:   "batch/v1",
		// 	DeployStatus: v1alpha1.StatusDeploying,
		// 	DeployTitle:  shell.GetTitle(),
		// }
		// installResult.ResourceList = append(installResult.ResourceList, shellInfo)
	}
	return installResult, jobs, nil
}

func (z *Install) InstallUseJob(name, namespace string, shellType types.ShellType) error {

	items := []v1alpha1.DeployItem{}
	jobs := []*batchv1.Job{}
	groups := []*v1alpha1.AppGroup{}
	// microApps := []*microapp.MicroApp{}
	root := z.pk.Root
	rootItem, rootjobs, err := z.createHelmJob(root, shellType, z.pk.Children)
	if err != nil {
		slog.Error("create helm job error", slog.String("error", err.Error()))
		return err
	}
	// rootMicro := convert.ToMicroApp(root)
	// if rootMicro != nil {
	// 	microApps = append(microApps, rootMicro)
	// }

	items = append(items, rootItem)
	jobs = append(jobs, rootjobs...)

	for _, child := range z.pk.Children {
		if !child.IsHelm() {
			continue
		}
		if child.Replicas == 0 {
			continue
		}
		childItem, childJobs, err := z.createHelmJob(child, shellType, []*types.PackageApp{})
		if err != nil {
			slog.Error("create child helm job error", slog.String("error", err.Error()))
			return err
		}
		items = append(items, childItem)
		jobs = append(jobs, childJobs...)
	}
	rootGroup := convert.ToAppGroup(root, items)
	rootGroup.Spec.UpgradingVersion = rootGroup.Spec.Version
	if err := z.applyParentGroup(rootGroup); err != nil {
		return err
	}

	// if (root.Parent != nil)  { //package app getLabel 判断了Parent is nil 就不需要设置parent
	// 	continue
	// }
	groups = append(groups, rootGroup)

	// sigClient, err := z.sdk.ToSigClient()
	// if err != nil {
	// 	slog.Error("create sig client error", slog.String("error", err.Error()))
	// 	return err
	// }

	for _, group := range groups {
		err = z.persistGroup(group)
		if err != nil {
			slog.Error("update group error", slog.String("error", err.Error()))
			return err
		}
	}

	// if !z.pk.IsHelm() {
	// 	for _, microApp := range microApps {
	// 		clone := microApp.DeepCopy()
	// 		_, err = controllerutil.CreateOrUpdate(z.sdk.Ctx, sigClient, clone, func() error {
	// 			clone.Spec = microApp.Spec
	// 			return nil
	// 		})
	// 		if err != nil {
	// 			slog.Error("create microapp error", slog.String("error", err.Error()))
	// 			return err
	// 		}
	// 	}
	// }
	for _, job := range jobs {
		_, err = z.sdk.ClientSet.BatchV1().Jobs(root.GetNamespace()).Create(z.sdk.Ctx, job, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				continue
			}
			slog.Error("create job error", slog.String("error", err.Error()))
			return err
		}
	}

	return nil
}

func (z *Install) IsHelm() bool {
	return z.pk.IsHelm()
}

func (z *Install) NeedHelmInstall() bool {
	// return z.pk.IsHelm()
	return z.pk.IsHelm() || z.pk.Root.HelmUrl != ""
}

func (z *Install) Install(name, namespace string) error {
	// go downStatic(z.pk.Root)
	if z.NeedHelmInstall() {
		//为啥helm 单独走一条线， 如果helmjob 当作一个helmchart安装的花，导致helm更新时候判断currentRelease只有一个job,比对不出来需要更新的资源
		//导致pvc每次都重建 数据丢失
		return z.InstallUseJob(name, namespace, types.ShellInstall)
	}

	helmchart := NewHelmChart(z.pk, types.ShellInstall)
	group, chart, err := helmchart.ToHelmChartWithGroup()
	if err != nil {
		return err
	}
	err = z.CreateOrUpdateGroup(namespace, name, group.Status.DeployItems)
	if err != nil {
		slog.Error("create or update group error", slog.String("error", err.Error()))
		return err
	}
	vals := map[string]interface{}{}
	// if z.pk.IsHelm() {
	vals, err = helmchart.GetValues()
	if err != nil {
		return err
	}
	// }
	helmApi := k8s.NewHelm(z.sdk)
	labels := z.fillLabelAndAnnation(chart)
	_, err = helmApi.Install(z.sdk.Ctx, chart, vals, name, namespace, labels)
	if err != nil {
		return err
	}
	return nil
}

func (z *Install) Upgrade(name, namespace string) error {
	if z.NeedHelmInstall() {
		return z.InstallUseJob(name, namespace, types.ShellUpgrade)
	}
	helmchart := NewHelmChart(z.pk, types.ShellUpgrade)
	group, chart, err := helmchart.ToHelmChartWithGroup()
	if err != nil {
		return err
	}
	err = z.CreateOrUpdateGroup(namespace, name, group.Status.DeployItems)
	if err != nil {
		slog.Error("create or update group error", slog.String("error", err.Error()))
		return err
	}
	helmApi := k8s.NewHelm(z.sdk)
	labels := z.fillLabelAndAnnation(chart)
	vals := map[string]interface{}{}
	if z.pk.IsHelm() {
		vals, err = helmchart.GetValues()
		if err != nil {
			return err
		}
	}
	_, err = helmApi.Upgrade(z.sdk.Ctx, chart, vals, name, namespace, labels)
	if err != nil {
		return err
	}
	return nil
}
func (z *Install) persistGroup(group *v1alpha1.AppGroup) error {
	namespace := group.Namespace
	fetchGroup, err := z.groupApi.GetAppGroup(namespace, group.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = z.groupApi.CreateGroup(namespace, group)
			if err != nil {
				slog.Error("create group error", slog.String("error", err.Error()))
				return err
			}
		}
		return err
	}
	oldVersion := fetchGroup.Spec.Version
	fetchGroup.Spec = group.Spec
	fetchGroup.Annotations = replaceAppGroupAnnotations(fetchGroup.Annotations, group.Annotations)
	fetchGroup.Labels = group.Labels

	fetchGroup.Spec.Version = oldVersion
	fetchGroup.Spec.UpgradingVersion = group.Spec.Version

	fetchGroup.Status.DeployItems = []v1alpha1.DeployItem{}
	fetchGroup.Status.DeployStatus = v1alpha1.StatusDeploying
	fetchGroup.Status.DeployItems = group.Status.DeployItems
	_, err = z.groupApi.UpdateAppGroup(namespace, fetchGroup)
	if err != nil {
		slog.Error("update group error", slog.String("error", err.Error()))
		return err
	}
	return nil
}

func (z *Install) CreateOrUpdateGroup(namespace, name string, items []v1alpha1.DeployItem) error {
	group, err := z.groupApi.GetAppGroup(namespace, name)
	if err != nil {
		group2 := helm.ToAppGroup(z.pk.Root, items)
		if err := z.applyParentGroup(group2); err != nil {
			return err
		}
		// group2.Spec.IsHelm = z.pk.IsHelm()
		_, err = z.groupApi.CreateGroup(namespace, group2)
		if err != nil {
			return err
		}
		return nil
	}
	if group != nil {
		group3 := helm.ToAppGroup(z.pk.Root, items)
		if err := z.applyParentGroup(group3); err != nil {
			return err
		}
		group.Annotations = replaceAppGroupAnnotations(group.Annotations, group3.Annotations)
		oldVersion := group.Spec.Version
		group.Spec = group3.Spec
		group.Spec.Version = oldVersion
		group.Spec.UpgradingVersion = group3.Spec.Version
		group.Status.DeployItems = []v1alpha1.DeployItem{}
		group.Status.DeployStatus = v1alpha1.StatusDeploying
		group.Status.DeployItems = append(group.Status.DeployItems, items...)
		_, err := z.groupApi.UpdateAppGroup(namespace, group)
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (z *Install) UnInstall(name, namespace string) error {
	helmApi := k8s.NewHelm(z.sdk)
	_, err := helmApi.UnInstall(name, namespace)
	if err != nil {
		return err
	}
	return nil
}

func (z *Install) Get(name, namespace string) (*release.Release, error) {
	helmApi := k8s.NewHelm(z.sdk)
	return helmApi.Info(name, namespace)
}

func (z *Install) fillLabelAndAnnation(chart *chart.Chart) map[string]string {
	return z.GetLabels()
}

// func (z *Install) GetAnnotations() map[string]string {
// 	helmConfig := z.pk.Root.Manifest.Platform.Helm
// 	anno := map[string]string{
// 		zpktypes.HELM_RELEASE_SOURCE:   "zpk",
// 		zpktypes.HELM_REPOSITORY_URL:   helmConfig.Repository,
// 		zpktypes.HELM_CHART_NAME:       helmConfig.ChartName,
// 		zpktypes.HELM_CHART_VERSION:    helmConfig.Version,
// 		zpktypes.HELM_ZPK_VERSION:      z.pk.Root.GetVersion(),
// 		zpktypes.HELM_ZPK_URL:          z.pk.Root.ZpkUrl,
// 		zpktypes.HELM_LOGO:             z.pk.Root.Manifest.Application.Icon,
// 		zpktypes.HELM_INDENTIFIE:       z.pk.Root.GetIdentifie(),
// 		zpktypes.HELM_APPLICATION_TYPE: z.pk.Root.Manifest.Application.Type,
// 		zpktypes.HELM_TITLE:            z.pk.Root.Manifest.Application.Name,
// 	}
// 	return anno
// }

func (z *Install) GetLabels() map[string]string {
	label := map[string]string{
		zpktypes.HELM_RELEASE_SOURCE: "zpk",
		zpktypes.HELM_INDENTIFIE:     z.pk.Root.GetIdentifie(),
	}
	return label
}

func replaceAppGroupAnnotations(current, desired map[string]string) map[string]string {
	var preservedAppGroupAnnotationKeys = []string{
		"w7.cc/bootstrap-owner",
		"w7.cc/domains",
		"w7.cc/ports",
		"w7.cc/default-domain",
		"w7.cc/create-svc",
	}

	result := make(map[string]string, len(desired)+len(preservedAppGroupAnnotationKeys))
	for key, value := range desired {
		result[key] = value
	}
	for _, key := range preservedAppGroupAnnotationKeys {
		if value, ok := current[key]; ok {
			result[key] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
