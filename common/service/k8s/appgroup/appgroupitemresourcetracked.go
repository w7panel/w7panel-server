package appgroup

import (
	"fmt"
	"log/slog"
	"reflect"

	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

type appGroupResourceTrackedScanner struct {
	APIVersion string
	Kind       string
	Get        func(namespace, name string) (metav1.Object, error)
	List       func(namespace string, selector labels.Selector) ([]metav1.Object, error)
	ItemStatus func(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus
	SyncGroup  func(evt *K8sResourceEvent, group *appgroupWrapper) error
}

type AppGroupItemResourceTracked struct {
	scanners []appGroupResourceTrackedScanner
}

func NewAppGroupItemResourceTracked() *AppGroupItemResourceTracked {
	return &AppGroupItemResourceTracked{}
}

func (d *AppGroupItemResourceTracked) RegisterScanner(scanner appGroupResourceTrackedScanner) {
	if scanner.Kind == "" || scanner.APIVersion == "" || scanner.Get == nil || scanner.List == nil {
		return
	}
	for i, existing := range d.scanners {
		if existing.Kind == scanner.Kind {
			d.scanners[i] = scanner
			return
		}
	}
	d.scanners = append(d.scanners, scanner)
}

func (d *AppGroupItemResourceTracked) RegisterWorkloads(deployment appsv1lister.DeploymentLister, statefulSet appsv1lister.StatefulSetLister, daemonSet appsv1lister.DaemonSetLister) {
	d.RegisterDeployment(deployment)
	d.RegisterStatefulSet(statefulSet)
	d.RegisterDaemonSet(daemonSet)
}

func (d *AppGroupItemResourceTracked) RegisterDeployment(lister appsv1lister.DeploymentLister) {
	if lister == nil {
		return
	}
	d.RegisterScanner(appGroupResourceTrackedScanner{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Get: func(namespace, name string) (metav1.Object, error) {
			return lister.Deployments(namespace).Get(name)
		},
		List: func(namespace string, selector labels.Selector) ([]metav1.Object, error) {
			items, err := lister.Deployments(namespace).List(selector)
			if err != nil {
				return nil, err
			}
			result := make([]metav1.Object, 0, len(items))
			for _, item := range items {
				result = append(result, item)
			}
			return result, nil
		},
		ItemStatus: func(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
			return workloadAppGroupResourceTrackedItemStatus(evt, obj)
		},
	})
}

func (d *AppGroupItemResourceTracked) RegisterStatefulSet(lister appsv1lister.StatefulSetLister) {
	if lister == nil {
		return
	}
	d.RegisterScanner(appGroupResourceTrackedScanner{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Get: func(namespace, name string) (metav1.Object, error) {
			return lister.StatefulSets(namespace).Get(name)
		},
		List: func(namespace string, selector labels.Selector) ([]metav1.Object, error) {
			items, err := lister.StatefulSets(namespace).List(selector)
			if err != nil {
				return nil, err
			}
			result := make([]metav1.Object, 0, len(items))
			for _, item := range items {
				result = append(result, item)
			}
			return result, nil
		},
		ItemStatus: func(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
			return workloadAppGroupResourceTrackedItemStatus(evt, obj)
		},
	})
}

func (d *AppGroupItemResourceTracked) RegisterDaemonSet(lister appsv1lister.DaemonSetLister) {
	if lister == nil {
		return
	}
	d.RegisterScanner(appGroupResourceTrackedScanner{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Get: func(namespace, name string) (metav1.Object, error) {
			return lister.DaemonSets(namespace).Get(name)
		},
		List: func(namespace string, selector labels.Selector) ([]metav1.Object, error) {
			items, err := lister.DaemonSets(namespace).List(selector)
			if err != nil {
				return nil, err
			}
			result := make([]metav1.Object, 0, len(items))
			for _, item := range items {
				result = append(result, item)
			}
			return result, nil
		},
		ItemStatus: func(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
			return workloadAppGroupResourceTrackedItemStatus(evt, obj)
		},
	})
}

func (d *AppGroupItemResourceTracked) RegisterIngress(lister networkingv1lister.IngressLister) {
	if lister == nil {
		return
	}
	d.RegisterScanner(appGroupResourceTrackedScanner{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "Ingress",
		Get: func(namespace, name string) (metav1.Object, error) {
			return lister.Ingresses(namespace).Get(name)
		},
		List: func(namespace string, selector labels.Selector) ([]metav1.Object, error) {
			items, err := lister.Ingresses(namespace).List(selector)
			if err != nil {
				return nil, err
			}
			result := make([]metav1.Object, 0, len(items))
			for _, item := range items {
				result = append(result, item)
			}
			return result, nil
		},
		SyncGroup: func(evt *K8sResourceEvent, group *appgroupWrapper) error {
			return syncAppGroupDomains(evt, group, lister)
		},
	})
}

func (d *AppGroupItemResourceTracked) GetAppGroupResourceTrackedFromRO(kind, namespace, name string) (metav1.Object, error) {
	scanner, ok := d.appGroupResourceTrackedScanner(kind)
	if !ok {
		return nil, fmt.Errorf("unknown appgroup resource tracked kind: %s", kind)
	}
	return scanner.Get(namespace, name)
}

func (d *AppGroupItemResourceTracked) handleAppGroupResourceTrackedEvent(evt *K8sResourceEvent, group *appgroupWrapper) error {
	if evt.EventType == "delete" {
		obj := &evt.ObjectMeta
		return d.HandleAppGroupResourceTracked(evt, group, obj, true)
	}
	resource, err := d.GetAppGroupResourceTrackedFromRO(evt.Kind, evt.Namespace, evt.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			obj := &evt.ObjectMeta
			return d.HandleAppGroupResourceTracked(evt, group, obj, true)
		}
		slog.Error("get appgroup resource tracked from ro error", "error", err)
		return nil
	}
	return d.HandleAppGroupResourceTracked(evt, group, resource, evt.EventType == "delete")
}

func (d *AppGroupItemResourceTracked) isAppGroupResourceTrackedKind(kind string) bool {
	_, ok := d.appGroupResourceTrackedScanner(kind)
	return ok
}

func (d *AppGroupItemResourceTracked) HandleAppGroupResourceTracked(evt *K8sResourceEvent, group *appgroupWrapper, obj metav1.Object, delete bool) error {
	scanner, _ := d.appGroupResourceTrackedScanner(evt.Kind)
	item := scannerItemStatus(scanner, evt, obj)
	return d.syncAppGroupResourceTrackedWrapper(group, item, delete)
}

func (d *AppGroupItemResourceTracked) syncAppGroupResourceTrackedWrapper(group *appgroupWrapper, item v1alpha1.AppGroupItemStatus, delete bool) error {
	if group == nil || !group.IsExists() {
		return nil
	}
	if delete {
		group.RemoveSyncedStatusItem(item)
	} else {
		group.SyncStatusItem(item)
	}
	return nil
}

func (d *AppGroupItemResourceTracked) syncAppGroupResourceTrackedDerivedState(evt *K8sResourceEvent, group *appgroupWrapper) error {
	if evt == nil {
		return nil
	}
	scanner, ok := d.appGroupResourceTrackedScanner(evt.Kind)
	if !ok || scanner.SyncGroup == nil {
		return nil
	}
	return scanner.SyncGroup(evt, group)
}

func (d *AppGroupItemResourceTracked) syncAllAppGroupResourceTrackedDerivedState(group *appgroupWrapper) error {
	for _, scanner := range d.appGroupResourceTrackedScanners() {
		if scanner.SyncGroup == nil {
			continue
		}
		if err := scanner.SyncGroup(nil, group); err != nil {
			return err
		}
	}
	return nil
}

func (d *AppGroupItemResourceTracked) scanAppGroupResourceTracked(wrapper *appgroupWrapper) (bool, error) {
	scanners := d.appGroupResourceTrackedScanners()
	if wrapper == nil || len(scanners) == 0 {
		return false, nil
	}
	group := wrapper.AppGroup
	oldSpec := group.Spec.DeepCopy()
	oldStatus := group.Status.DeepCopy()
	selectors := []labels.Selector{labels.Everything()}
	seen := map[string]struct{}{}
	trackedKinds := map[string]struct{}{}
	for _, scanner := range scanners {
		trackedKinds[appGroupResourceTrackedKindKey(scanner.APIVersion, scanner.Kind)] = struct{}{}
		for _, selector := range selectors {
			items, err := scanner.List(group.Namespace, selector)
			if err != nil {
				return false, err
			}
			for _, item := range items {
				d.upsertScannedResource(wrapper, scanner.APIVersion, scanner.Kind, item, seen)
			}
		}
	}
	pruneMissingAppGroupResourceTracked(wrapper, trackedKinds, seen)
	return !reflect.DeepEqual(*oldSpec, group.Spec) || !reflect.DeepEqual(*oldStatus, group.Status), nil
}

func (d *AppGroupItemResourceTracked) appGroupResourceTrackedScanners() []appGroupResourceTrackedScanner {
	return d.scanners
}

func (d *AppGroupItemResourceTracked) appGroupResourceTrackedScanner(kind string) (appGroupResourceTrackedScanner, bool) {
	for _, scanner := range d.appGroupResourceTrackedScanners() {
		if scanner.Kind == kind {
			return scanner, true
		}
	}
	return appGroupResourceTrackedScanner{}, false
}

func (d *AppGroupItemResourceTracked) upsertScannedResource(group *appgroupWrapper, apiVersion, kind string, obj metav1.Object, seen map[string]struct{}) {
	scanner, _ := d.appGroupResourceTrackedScanner(kind)
	if !resourceVisibleInGroup(obj, group.Name) {
		return
	}
	key := appGroupResourceTrackedItemKey(apiVersion, kind, obj.GetName())
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	group.SyncStatusItem(scannerItemStatus(scanner, &K8sResourceEvent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       kind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		},
	}, obj))
}

func pruneMissingAppGroupResourceTracked(group *appgroupWrapper, trackedKinds, seen map[string]struct{}) {
	for _, item := range group.Status.Items {
		if _, tracked := trackedKinds[appGroupResourceTrackedKindKey(item.ApiVersion, item.Kind)]; tracked {
			if _, ok := seen[appGroupResourceTrackedItemKey(item.ApiVersion, item.Kind, item.Name)]; !ok {
				group.RemoveSyncedStatusItem(item)
				continue
			}
		}
	}
}

func appGroupResourceTrackedKindKey(apiVersion, kind string) string {
	return apiVersion + "/" + kind
}

func appGroupResourceTrackedItemKey(apiVersion, kind, name string) string {
	return appGroupResourceTrackedKindKey(apiVersion, kind) + "/" + name
}

func scannerItemStatus(scanner appGroupResourceTrackedScanner, evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
	if scanner.ItemStatus != nil {
		return scanner.ItemStatus(evt, obj)
	}
	return appGroupResourceTrackedItemStatus(evt, obj)
}

func workloadAppGroupResourceTrackedItemStatus(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
	wrapper := NewWorkloadWrapper(obj)
	if wrapper == nil {
		return appGroupResourceTrackedItemStatus(evt, obj)
	}
	return wrapper.ToItemStatus()
}

func appGroupResourceTrackedItemStatus(evt *K8sResourceEvent, obj metav1.Object) v1alpha1.AppGroupItemStatus {
	deployStatus := v1alpha1.StatusDeployed
	return v1alpha1.AppGroupItemStatus{
		Kind:              evt.Kind,
		ApiVersion:        evt.APIVersion,
		Name:              obj.GetName(),
		Title:             resourceTitle(obj),
		Ready:             true,
		CreationTimestamp: obj.GetCreationTimestamp(),
		DeployStatus:      deployStatus,
	}
}

func resourceTitle(obj metav1.Object) string {
	annotations := obj.GetAnnotations()
	if annotations != nil {
		if annotations["w7.cc/title"] != "" {
			return annotations["w7.cc/title"]
		}
		if annotations["title"] != "" {
			return annotations["title"]
		}
	}
	return obj.GetName()
}
