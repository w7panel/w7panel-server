package microapp

import (
	"context"
	"fmt"
	"maps"
	"strings"

	appgroupv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	microappv1 "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const resourceGroupLabel = "w7.cc/group-name"

// MicroAppController keeps one MicroApp's reverse dependency labels aligned
// with the AppGroup identified by w7.cc/group-name.
type MicroAppController struct {
	client.Client
}

func SetupMicroAppController(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("microapp").
		For(&microappv1.MicroApp{}).
		Complete(&MicroAppController{Client: mgr.GetClient()})
}

func (r *MicroAppController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	item := &microappv1.MicroApp{}
	if err := r.Get(ctx, req.NamespacedName, item); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !item.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}
	groupName := strings.TrimSpace(item.Labels[resourceGroupLabel])
	if groupName == "" {
		return ctrl.Result{}, nil
	}
	group := &appgroupv1.AppGroup{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: item.Namespace, Name: groupName}, group); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	before := item.DeepCopy()
	changed, err := syncDependencyLabels(item, group)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !changed {
		return ctrl.Result{}, nil
	}
	if err := r.Patch(ctx, item, client.MergeFrom(before)); err != nil {
		return ctrl.Result{}, fmt.Errorf("sync dependency labels to MicroApp %s/%s: %w", item.Namespace, item.Name, err)
	}
	return ctrl.Result{}, nil
}

func syncDependencyLabels(item *microappv1.MicroApp, group *appgroupv1.AppGroup) (bool, error) {
	desired, err := appgroupv1.DesiredDependencyLabels(group.Spec.Dependencies)
	if err != nil {
		return false, err
	}
	before := maps.Clone(item.Labels)
	for key := range item.Labels {
		if strings.HasPrefix(key, appgroupv1.DependencyLabelPrefix) {
			delete(item.Labels, key)
		}
	}
	if item.Labels == nil && len(desired) > 0 {
		item.Labels = map[string]string{}
	}
	maps.Copy(item.Labels, desired)
	return !maps.Equal(before, item.Labels), nil
}
