package appgroup

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"

	// "github.com/w7panel/w7panel/common/service/k8s/zpk"
	"github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	microapp "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	v1alpha1Lister "github.com/w7panel/w7panel/k8s/pkg/client/appgroup/listers/appgroup/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	k8stypes "k8s.io/apimachinery/pkg/types"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	batchv1lister "k8s.io/client-go/listers/batch/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

const (
	appGroupFinalizer       = "w7panel.w7.com/finalizer"
	legacyAppGroupFinalizer = "w7panel.w7.com/finalizers"
)

type appGroupCleanupRetryError struct {
	err error
}

func (e *appGroupCleanupRetryError) Error() string {
	return e.err.Error()
}

func (e *appGroupCleanupRetryError) Unwrap() error {
	return e.err
}

func newAppGroupCleanupRetryError(err error) error {
	return &appGroupCleanupRetryError{err: err}
}

func shouldHandleAppGroupEvent(group *v1alpha1.AppGroup) bool {
	return group.Namespace == "default" || group.DeletionTimestamp != nil
}

var oldAppGroupGVR = schema.GroupVersionResource{
	Group:    "appgroup.w7.cc",
	Version:  "v1alpha1",
	Resource: "appgroups",
}

type WorkloadManager struct {
	groupApi                    *AppGroupApi
	helm                        *k8s.Helm
	sdk                         *k8s.Sdk
	AppGroupLister              v1alpha1Lister.AppGroupLister
	DeploymentLister            appsv1lister.DeploymentLister
	DaemonSetLister             appsv1lister.DaemonSetLister
	StatefulSetLister           appsv1lister.StatefulSetLister
	JobLister                   batchv1lister.JobLister
	SecretLister                corev1lister.SecretLister
	IngressLister               networkingv1lister.IngressLister
	helmworkload                *HelmWorkload
	AppGroupItemResourceTracked *AppGroupItemResourceTracked
}

func NewWorkLoadTestManager() *WorkloadManager {
	sdk := k8s.NewK8sClientInner()
	helm := k8s.NewHelm(sdk)
	groupApi, err := NewAppGroupApi(sdk)
	if err != nil {
		panic(fmt.Errorf("init appgroup api error: %v", err))
	}
	manager := NewWorkloadManager(groupApi, helm)
	return manager

}

func NewWorkloadManager(groupApi *AppGroupApi, helm *k8s.Helm) *WorkloadManager {
	return &WorkloadManager{
		groupApi:                    groupApi,
		helm:                        helm,
		sdk:                         groupApi.sdk,
		AppGroupItemResourceTracked: NewAppGroupItemResourceTracked(),
	}
}

func (d *WorkloadManager) SetGroupLister(lister v1alpha1Lister.AppGroupLister) {
	d.groupApi.SetLister(lister)
	d.AppGroupLister = lister
}

func NewWorkloadManagerWithLister(groupApi *AppGroupApi, helm *k8s.Helm,
	groupLister v1alpha1Lister.AppGroupLister,
	deployment appsv1lister.DeploymentLister,
	statefulset appsv1lister.StatefulSetLister,
	daemonset appsv1lister.DaemonSetLister,
	job batchv1lister.JobLister,
	secret corev1lister.SecretLister,
) *WorkloadManager {
	manager := &WorkloadManager{
		groupApi:                    groupApi,
		helm:                        helm,
		sdk:                         groupApi.sdk,
		AppGroupLister:              groupLister,
		DeploymentLister:            deployment,
		StatefulSetLister:           statefulset,
		DaemonSetLister:             daemonset,
		JobLister:                   job,
		SecretLister:                secret,
		AppGroupItemResourceTracked: NewAppGroupItemResourceTracked(),
	}
	manager.AppGroupItemResourceTracked.RegisterWorkloads(deployment, statefulset, daemonset)
	return manager
}

func (d *WorkloadManager) GetGroupName(ds WorkloadWrapperInterface) string {
	name := ds.Name()
	parentName := ds.ReleaseName()
	if parentName != "" {
		name = parentName
	}
	lb := ds.Labels()
	managerBy, ok := lb["app.kubernetes.io/managed-by"]
	if ok {
		if managerBy != "Helm" {
			deployment, err := d.DeploymentLister.Deployments(ds.Namespace()).Get(managerBy)
			if err != nil {
				slog.Error("get deployment error", "error", err)
				return name
			}
			releaseName, ok := deployment.Labels["app.kubernetes.io/instance"]
			if ok {
				name = releaseName
			}
		}
	}
	anno := ds.Annotations()
	if releaseName, ok := anno["meta.helm.sh/release-name"]; ok {
		name = releaseName
	}
	return name
}

func (d *WorkloadManager) GetFromRO(kind, namespace, name string) (WorkloadWrapperInterface, error) {

	switch kind {
	case "Deployment":
		deployment, err := d.DeploymentLister.Deployments(namespace).Get(name)
		if err != nil {
			return nil, err
		}
		return NewWorkloadWrapper(deployment), nil
	case "StatefulSet":
		statefulset, err := d.StatefulSetLister.StatefulSets(namespace).Get(name)
		if err != nil {
			return nil, err
		}
		return NewWorkloadWrapper(statefulset), nil
	case "DaemonSet":
		daemonset, err := d.DaemonSetLister.DaemonSets(namespace).Get(name)
		if err != nil {
			return nil, err
		}
		return NewWorkloadWrapper(daemonset), nil
	case "Job":
		job, err := d.JobLister.Jobs(namespace).Get(name)
		if err != nil {
			return nil, err
		}
		return NewWorkloadWrapper(job), nil
	default:

		return nil, fmt.Errorf("unknown kind: %s", kind)
	}
}

func (d *WorkloadManager) GetSecretFromRO(kind, namespace, name string) (*corev1.Secret, error) {
	return d.SecretLister.Secrets(namespace).Get(name)
}

func (d *WorkloadManager) GetAppGroupFromRO(namespace, name string) (*v1alpha1.AppGroup, error) {
	group, err := d.AppGroupLister.AppGroups(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	// Informer lister 返回的对象属于共享缓存，协调过程需要修改 finalizer，
	// 必须先复制，避免污染缓存或与其他 worker 产生数据竞争。
	return group.DeepCopy(), nil
}

func (d *WorkloadManager) GetAppGroupWrapper(ds WorkloadWrapperInterface) *appgroupWrapper {
	name := d.GetGroupName(ds)
	spec := appv1.AppGroupSpec{
		Type:        appv1.CUSTOM,
		Identifie:   ds.Identifie(),
		Version:     "",
		Logo:        "",
		Description: "",
		ZpkUrl:      "",
		Suffix:      ds.Labels()["w7.cc/suffix"],
		Title:       ds.Title(),
	}

	return d.groupApi.GetAppGroupWrapper(ds.Namespace(), name, spec)
}

func (d *WorkloadManager) HandleQueue(key interface{}) error {
	evt, err := ParseEvent(key)
	if err != nil {
		slog.Error("parse key error", "error", err)
		return nil
	}

	if evt.Kind == "AppGroup" {
		group, err := d.GetAppGroupFromRO(evt.Namespace, evt.Name)
		if err != nil {
			slog.Error("get from ro error", "error", err)
			return nil
		}
		if !shouldHandleAppGroupEvent(group) {
			return nil
		}
		return d.HandleAppGroup(group, false, evt.IsInit)
	}

	if evt.Namespace != "default" {
		return nil
	}
	// if (ev)
	if evt.Kind == "Event" {
		return nil
	}
	if evt.Kind == "Secret" {
		secret, err := d.GetSecretFromRO(evt.Kind, evt.Namespace, evt.Name)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			// slog.Error("get from ro error", "error", err)
			return nil
		}
		go func() {
			if helper.IsK3kVirtual() {
				k3k.SyncSecretHttp(secret) // virtual agent 可能在启动中导致secret 没有接收到webhook事件
			}
		}()
		return d.HandleSecret(secret, false)
	}
	if d.AppGroupItemResourceTracked.isAppGroupResourceTrackedKind(evt.Kind) {
		err = d.handleAppGroupResourceTrackedEvent(evt, d.GetAppGroupResourceTrackedGroupWrappers(evt))
		slog.Info("handleAppGroupResourceTrackedEvent complete", "error", err, "event", evt)
	}

	if isWorkloadKind(evt.Kind) {
		ds, err := d.GetFromRO(evt.Kind, evt.Namespace, evt.Name)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				workload := NewWorkloadFromEvent(evt)
				return d.HandleWorkload(NewWorkloadWrapper(workload), true)
			}
			slog.Error("get from ro error", "error", err)
			return nil
		}
		return d.HandleWorkload(ds, evt.EventType == "delete")
	}

	return nil
}

func (d *WorkloadManager) HandleWorkload(ds WorkloadWrapperInterface, delete bool) error {
	slog.Debug("handle workload", "workload", ds.Name())

	itemStatus := ds.ToItemStatus()
	if itemStatus.Kind != "Job" {
		d.fixSvc(ds, delete)
	}
	if delete {
		return nil
	}

	group := d.GetAppGroupWrapper(ds)
	//job 如果 获取到的 group不存在， 跳过
	if itemStatus.Kind == "Job" {
		if group == nil || !group.exists {
			return nil
		}
	}

	//兼容 operator 管理的应用 同步到appgroup
	//非 helm 安装的单应用 这里会跳过，d.groupApi.Persist(group) 会新建 appgroup出来
	managerBy, ok := ds.Labels()["app.kubernetes.io/managed-by"]
	if group == nil || (ds.IsHelm() && !group.exists) || (ok && managerBy != "Helm" && !group.exists) {
		return nil
	}

	group.FixDeployItem(itemStatus)
	_, err := d.groupApi.Persist(group)
	if err != nil {
		slog.Error("update group error", "error", err)
		return err
	}
	if err := d.ensureWorkloadGroupNameLabel(ds, group.Name); err != nil {
		slog.Error("patch workload group name label error", "error", err, "kind", ds.Kind(), "namespace", ds.Namespace(), "name", ds.Name(), "groupName", group.Name)
		return err
	}
	if itemStatus.Kind != "Job" {
		notifyInstalledForReadyGroups(group)
	}

	return nil

}

func (d *WorkloadManager) ensureWorkloadGroupNameLabel(workload WorkloadWrapperInterface, groupName string) error {
	if workload == nil || groupName == "" || workload.Labels()[groupNameKey] != "" {
		return nil
	}
	patch, err := json.Marshal(map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": map[string]string{groupNameKey: groupName},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal workload group name label patch: %w", err)
	}

	ctx := d.sdk.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	patchOptions := metav1.PatchOptions{FieldManager: "w7panel-appgroup-controller"}
	switch workload.Kind() {
	case "Deployment":
		_, err = d.sdk.ClientSet.AppsV1().Deployments(workload.Namespace()).Patch(ctx, workload.Name(), k8stypes.MergePatchType, patch, patchOptions)
	case "StatefulSet":
		_, err = d.sdk.ClientSet.AppsV1().StatefulSets(workload.Namespace()).Patch(ctx, workload.Name(), k8stypes.MergePatchType, patch, patchOptions)
	case "DaemonSet":
		_, err = d.sdk.ClientSet.AppsV1().DaemonSets(workload.Namespace()).Patch(ctx, workload.Name(), k8stypes.MergePatchType, patch, patchOptions)
	case "Job":
		_, err = d.sdk.ClientSet.BatchV1().Jobs(workload.Namespace()).Patch(ctx, workload.Name(), k8stypes.MergePatchType, patch, patchOptions)
	default:
		return fmt.Errorf("unsupported workload kind for group name label: %s", workload.Kind())
	}
	if err != nil {
		return fmt.Errorf("patch %s %s/%s group name label: %w", workload.Kind(), workload.Namespace(), workload.Name(), err)
	}
	return nil
}

func (d *WorkloadManager) handleAppGroupResourceTrackedEvent(evt *K8sResourceEvent, groups []*appgroupWrapper) error {
	for _, group := range groups {
		if err := d.AppGroupItemResourceTracked.handleAppGroupResourceTrackedEvent(evt, group); err != nil {
			return err
		}
		if err := d.AppGroupItemResourceTracked.syncAppGroupResourceTrackedDerivedState(evt, group); err != nil {
			return err
		}
		if group != nil && group.IsChange() {
			if _, err := d.groupApi.Persist(group); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *WorkloadManager) GetAppGroupResourceTrackedGroupWrappers(resource interface{}) []*appgroupWrapper {
	obj, ok := resource.(metav1.Object)
	if !ok {
		return nil
	}
	groupNames := getResourceGroupNames(obj)
	groups := make([]*appgroupWrapper, 0, len(groupNames))
	for _, groupName := range groupNames {
		groups = append(groups, d.groupApi.GetAppGroupWrapper(obj.GetNamespace(), groupName, v1alpha1.AppGroupSpec{}))
	}
	return groups
}

func notifyInstalledForReadyGroups(group *appgroupWrapper) {
	for current := group; current != nil; current = current.parent {
		if !current.Status.Ready {
			continue
		}
		if err := NotifyInstalled(current.AppGroup); err != nil {
			slog.Error("notify installed error", "error", err)
		}
	}
}

// secret 只修改spec 中logo isHelm 字段
func (d *WorkloadManager) HandleSecret(secret *corev1.Secret, delete bool) error {
	if secret == nil {
		return fmt.Errorf("secret is nil")
	}
	// if helper.IsChildAgent() && helper.IsK3kVirtual() && secret.Type != "helm.sh/release.v1" {
	// 	k3k.SyncSecretHttp(secret)
	// }
	// if !helper.IsChildAgent() {// 因为只watch default namespace 使用webhook 处理
	// 	k3k.SyncToChildSecret(secret) // 主集群同步到子集群的secret
	// }
	if secret.Type != "helm.sh/release.v1" {
		return nil
	}
	if delete {
		slog.Debug("delete secret", slog.String("name", secret.Name))
		return nil
	}
	// 全量同步helm 应用组信息
	err := d.helmworkload.Sync()
	if err != nil {
		slog.Error("helm sync error", "error", err)
		return err
	}
	return err

}

func (d *WorkloadManager) cleanGroupChildren(group *v1alpha1.AppGroup) error {
	req, err := labels.NewRequirement("w7.cc/parent", selection.In, []string{group.Name})
	if err != nil {
		slog.Error("failed to create requirement", slog.String("error", err.Error()))
		return err
	}
	mlabel := labels.NewSelector().Add(*req)
	children, err := d.groupApi.lister.AppGroups(group.Namespace).List(mlabel)
	if err != nil {
		slog.Error("failed to list children", slog.String("error", err.Error()))
		return err
	}

	hasChildren := false
	for _, child := range children {
		if child.DeletionTimestamp == nil {
			hasChildren = true
			err = d.groupApi.DeleteAppGroup(child.Namespace, child.Name)
			if err != nil {
				slog.Error("DeleteAppGroup err", "group", child.Name, "err", err)
			}
		}
	}
	if !hasChildren {
		slog.Debug("no children need to delete", "group name", group.Name, "namespace", group.Namespace)
		if !removeManagedAppGroupFinalizers(group) {
			return nil
		}
		_, err := d.groupApi.UpdateAppGroup(group.Namespace, group)
		if err != nil {
			slog.Error("update group error", "error", err)
			return err
		}
	}

	return nil
}

func (d *WorkloadManager) HandleAppGroup(group *v1alpha1.AppGroup, delete bool, isInit bool) error {
	if group.DeletionTimestamp != nil {
		// d.deleteOldAppGroup(group)
		if err := d.cleanAppGroup(group); err != nil {
			return newAppGroupCleanupRetryError(err)
		}
		parentName, isChild := group.Labels["w7.cc/parent"]
		if isChild {
			if removeManagedAppGroupFinalizers(group) {
				_, err := d.groupApi.UpdateAppGroup(group.Namespace, group)
				if err != nil {
					slog.Error("update group error", "error", err)
					return err
				}
			}
			parentGroup, err := d.GetAppGroupFromRO(group.Namespace, parentName)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					return nil
				}
				slog.Debug("cannot find parent group", slog.String("parentName", parentName), slog.String("error", err.Error()))
				return err
			}
			return d.cleanGroupChildren(parentGroup)
		} else {
			go func() {
				if err := NotifyDeleted(group); err != nil {
					slog.Warn("notify appgroup deleted failed", "namespace", group.Namespace, "name", group.Name, "error", err)
				}
			}()

			return d.cleanGroupChildren(group)
		}
	}

	changed := false
	if group.Spec.Suffix == "" {
		group.Spec.Suffix = group.Name
		changed = true
	}

	if group.Status.Ready {
		if NeedNotifyInstalled(group) {
			err := NotifyInstalled(group)
			if err != nil && err.Error() == "appgroup notify error" {
				// d.groupApi.DeleteAppGroup(group.Namespace, group.Name)
				return nil
			}
			if err == nil {
				group.Annotations["w7.cc/notify-installed"] = "true"
				changed = true
			}
		}

	}
	wrapper := NewAppGroupWrapper(group, true)
	if scanChanged, err := d.AppGroupItemResourceTracked.scanAppGroupResourceTracked(wrapper); err != nil {
		slog.Error("scan appgroup resource tracked error", "error", err)
	} else if scanChanged {
		changed = true
	}
	if err := d.AppGroupItemResourceTracked.syncAllAppGroupResourceTrackedDerivedState(wrapper); err != nil {
		slog.Error("sync appgroup resource tracked derived state error", "error", err)
	} else if wrapper.IsChange() {
		changed = true
	}
	if changed {
		_, err := d.groupApi.UpdateAppGroup(group.Namespace, group)
		if err != nil {
			slog.Error("update group error", "error", err)
			return err
		}
	}

	if isInit {
		// DownStatic(group)
	}

	return nil
}

func removeManagedAppGroupFinalizers(group *v1alpha1.AppGroup) bool {
	if group == nil || len(group.Finalizers) == 0 {
		return false
	}
	finalizers := group.Finalizers[:0]
	for _, finalizer := range group.Finalizers {
		if finalizer == appGroupFinalizer || finalizer == legacyAppGroupFinalizer {
			continue
		}
		finalizers = append(finalizers, finalizer)
	}
	changed := len(finalizers) != len(group.Finalizers)
	group.Finalizers = finalizers
	return changed
}

func skipHelmCleanup(group *v1alpha1.AppGroup) bool {
	return group.Name == "longhorn" || group.Name == "w7panel-offline"
}

func (d *WorkloadManager) cleanHelm(group *v1alpha1.AppGroup) error {
	if skipHelmCleanup(group) { //LonghornUpgrade 之前版本有bug 暂时不清理
		return nil
	}
	_, err := d.helm.UnInstall(group.Name, group.Namespace)
	if err != nil {
		return fmt.Errorf("uninstall helm release %s/%s: %w", group.Namespace, group.Name, err)
	}
	return nil
}

func (d *WorkloadManager) cleanAppGroup(group *appv1.AppGroup) error {
	defer helper.CleanStaticDir(group.Name)

	helmInfo, err := d.helm.Info(group.Name, group.Namespace)
	if err != nil {
		slog.Error("failed to get helm info", slog.String("error", err.Error()))
	}
	if helmInfo != nil { //如果云端应用也是helm 资源
		// vm metrics opertor 会监听资源删除 如果helm uninstall 最后执行 会导致资源无法删除 helm清理不干净
		slog.Info("start delete helm ")
		err := d.cleanHelm(group)
		if err != nil {
			return err
		}
	}

	slog.Info("start delete workload")
	for _, workload := range group.Status.Items {
		if err := d.deleteAppGroupStatusItemResource(group.Namespace, workload); err != nil {
			slog.Error("failed to delete appgroup status item",
				slog.String("apiVersion", workload.ApiVersion),
				slog.String("kind", workload.Kind),
				slog.String("name", workload.Name),
				slog.String("error", err.Error()),
			)
		}
		//清理service
		err := d.sdk.ClientSet.CoreV1().Services(group.Namespace).Delete(context.TODO(), workload.Name, metav1.DeleteOptions{})
		if err != nil {
			slog.Error("failed to delete service", slog.String("error", err.Error()))
		}
		err = d.sdk.ClientSet.CoreV1().Services(group.Namespace).Delete(context.TODO(), workload.Name+"-lb", metav1.DeleteOptions{})
		if err != nil {
			slog.Error("failed to delete servicelb", slog.String("error", err.Error()))
		}
	}

	jobs, err := d.sdk.ClientSet.BatchV1().Jobs(group.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("w7.cc/group-name=%s", group.Name)})
	if err != nil {
		slog.Error("failed to list jobs", slog.String("error", err.Error()))
		// return
	}
	if jobs != nil {
		for _, job := range jobs.Items {
			d.sdk.ClientSet.BatchV1().Jobs(group.Namespace).Delete(context.TODO(), job.Name, metav1.DeleteOptions{})
		}
	}

	cronJobs, err := d.sdk.ClientSet.BatchV1beta1().CronJobs(group.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: fmt.Sprintf("w7.cc/group-name=%s", group.Name)})
	if err != nil {
		slog.Error("failed to list cronjobs", slog.String("error", err.Error()))
		// return
	}
	if cronJobs != nil {
		for _, cronJob := range cronJobs.Items {
			d.sdk.ClientSet.BatchV1beta1().CronJobs(group.Namespace).Delete(context.TODO(), cronJob.Name, metav1.DeleteOptions{})
		}
	}

	slog.Info("start delete ingress")
	ingressLabel := fmt.Sprintf("group=%s", group.Name)
	ingress, err := d.sdk.ClientSet.NetworkingV1().Ingresses(group.Namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: ingressLabel})
	if err != nil {
		slog.Error("failed to list ingress", slog.String("error", err.Error()))
		return nil
	}
	for _, item := range ingress.Items {
		slog.Info("delete ingress", slog.String("ingressName", item.Name))
		d.sdk.ClientSet.NetworkingV1().Ingresses(group.Namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	}

	sigClient, err := d.sdk.ToSigClient()
	if err != nil {
		slog.Error("failed to get sig client", slog.String("error", err.Error()))
		return nil
	}
	mc := &microapp.MicroApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group.Name,
			Namespace: group.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "MicroApp",
			APIVersion: "w7panel.w7.com/v1alpha1",
		},
	}
	err = sigClient.Delete(d.sdk.Ctx, mc, &sigclient.DeleteOptions{})
	if err != nil {
		slog.Error("failed to delete microapp", slog.String("error", err.Error()))
	}
	return nil
}

func (d *WorkloadManager) deleteAppGroupStatusItemResource(namespace string, item appv1.AppGroupItemStatus) error {
	if item.Name == "" || item.Kind == "" {
		return nil
	}

	switch item.Kind {
	case "Deployment":
		return d.sdk.ClientSet.AppsV1().Deployments(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "DaemonSet":
		return d.sdk.ClientSet.AppsV1().DaemonSets(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "StatefulSet":
		return d.sdk.ClientSet.AppsV1().StatefulSets(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "CronJob":
		if item.ApiVersion == "batch/v1" {
			return d.sdk.ClientSet.BatchV1().CronJobs(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
		}
		return d.sdk.ClientSet.BatchV1beta1().CronJobs(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "Job":
		return d.sdk.ClientSet.BatchV1().Jobs(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "Ingress":
		return d.sdk.ClientSet.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "Service":
		return d.sdk.ClientSet.CoreV1().Services(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "Secret":
		return d.sdk.ClientSet.CoreV1().Secrets(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "ConfigMap":
		return d.sdk.ClientSet.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "PersistentVolumeClaim":
		return d.sdk.ClientSet.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	case "ServiceAccount":
		return d.sdk.ClientSet.CoreV1().ServiceAccounts(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
	default:
		return d.deleteAppGroupStatusItemWithDynamicClient(namespace, item)
	}
}

func (d *WorkloadManager) deleteAppGroupStatusItemWithDynamicClient(namespace string, item appv1.AppGroupItemStatus) error {
	if item.ApiVersion == "" || d.sdk == nil || d.sdk.DynamicClient() == nil {
		return nil
	}
	mapping, err := d.sdk.GetRestMapping(item.ApiVersion, item.Kind)
	if err != nil {
		return err
	}
	return d.sdk.DynamicClient().Resource(mapping.Resource).Namespace(namespace).Delete(context.TODO(), item.Name, metav1.DeleteOptions{})
}

func isWorkloadKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job":
		return true
	default:
		return false
	}
}
