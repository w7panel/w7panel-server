package appgroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
)

func TestEventQueueOnUpdateRemovesOldAppGroupResourceTrackedGroups(t *testing.T) {
	queue := NewDefaultEventQueue(func(key interface{}) error {
		return nil
	})
	defer queue.queue.ShutDown()

	oldIngress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
			Labels: map[string]string{
				"group": "old-group",
			},
		},
	}
	newIngress := oldIngress.DeepCopy()
	newIngress.Labels = map[string]string{
		"group": "new-group",
	}

	queue.OnUpdate(oldIngress, newIngress)

	oldKey, shutdown := queue.queue.Get()
	assert.False(t, shutdown)
	defer queue.queue.Done(oldKey)
	oldEvent, err := ParseEvent(oldKey)
	assert.NoError(t, err)
	assert.Equal(t, "delete", oldEvent.EventType)
	assert.Equal(t, []string{"old-group"}, getResourceGroupNames(oldEvent))

	newKey, shutdown := queue.queue.Get()
	assert.False(t, shutdown)
	defer queue.queue.Done(newKey)
	newEvent, err := ParseEvent(newKey)
	assert.NoError(t, err)
	assert.Equal(t, "update", newEvent.EventType)
	assert.Equal(t, []string{"new-group"}, getResourceGroupNames(newEvent))
}

func TestScanAppGroupResourceTrackedPrunesMissingTrackedItems(t *testing.T) {
	manager := NewAppGroupItemResourceTracked()
	manager.RegisterScanner(appGroupResourceTrackedScanner{
		APIVersion: "networking.k8s.io/v1",
		Kind:       "Ingress",
		Get: func(namespace, name string) (metav1.Object, error) {
			return nil, nil
		},
		List: func(namespace string, selector labels.Selector) ([]metav1.Object, error) {
			return []metav1.Object{
				&networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kept",
						Namespace: namespace,
						Labels: map[string]string{
							"group": "demo",
						},
					},
				},
			}, nil
		},
	})
	group := &v1alpha1.AppGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: v1alpha1.AppGroupStatus{
			Items: []v1alpha1.AppGroupItemStatus{
				{ApiVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "kept", Ready: true},
				{ApiVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "stale", Ready: true},
				{ApiVersion: "apps/v1", Kind: "Deployment", Name: "workload", Ready: true},
			},
		},
	}

	changed, err := manager.scanAppGroupResourceTracked(NewAppGroupWrapper(group, true))

	assert.NoError(t, err)
	assert.True(t, changed)
	assert.ElementsMatch(t, []v1alpha1.AppGroupItemStatus{
		{
			ApiVersion:        "networking.k8s.io/v1",
			Kind:              "Ingress",
			Name:              "kept",
			Title:             "kept",
			Ready:             true,
			CreationTimestamp: metav1.Time{},
			DeployStatus:      v1alpha1.StatusDeployed,
		},
		{ApiVersion: "apps/v1", Kind: "Deployment", Name: "workload", Ready: true},
	}, group.Status.Items)
}

func TestScanAppGroupResourceTrackedUsesWorkloadScannerStatus(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-workload",
			Namespace: "default",
			Labels: map[string]string{
				"group": "demo",
			},
			Annotations: map[string]string{
				"title": "Demo Workload",
			},
			CreationTimestamp: metav1.Unix(10, 0),
		},
		Status: appsv1.DeploymentStatus{
			Replicas:      1,
			ReadyReplicas: 1,
		},
	}
	assert.NoError(t, indexer.Add(deployment))

	manager := NewAppGroupItemResourceTracked()
	manager.RegisterDeployment(appsv1lister.NewDeploymentLister(indexer))
	group := &v1alpha1.AppGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
	}

	changed, err := manager.scanAppGroupResourceTracked(NewAppGroupWrapper(group, true))

	assert.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, []v1alpha1.AppGroupItemStatus{
		{
			ApiVersion:        "apps/v1",
			Kind:              "Deployment",
			Name:              "demo-workload",
			Title:             "Demo Workload",
			Ready:             true,
			CreationTimestamp: metav1.Unix(10, 0),
			DeployStatus:      v1alpha1.StatusDeployed,
		},
	}, group.Status.Items)
}
