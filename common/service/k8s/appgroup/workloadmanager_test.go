package appgroup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/w7panel/w7panel/common/service/k8s"
	appgroupv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	appgrouplister "github.com/w7panel/w7panel/k8s/pkg/client/appgroup/listers/appgroup/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type mockWorkloadWrapper struct {
	isHelm bool
	name   string
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestWorkloadManager_Handle(t *testing.T) {

	// manager := NewWorkLoadTestManager()
	// deployment, err := manager.sdk.ClientSet.AppsV1().Deployments("default").Get(manager.sdk.Ctx, "test1-ypyvijxs", metav1.GetOptions{})
	// if err != nil {
	// 	t.Errorf("Error getting deployment: %v", err)
	// }
	// wrapper := NewWorkloadWrapper(deployment)
	// manager.Handle(wrapper, false)

	// 验证AppGroupApi的Persist方法是否被调用
	// 验证AppGroupApi的Persist方法是否返回错误
}

func TestWorkloadManager_HandleJob(t *testing.T) {

	// manager := NewWorkLoadTestManager()
	// deployment, err := manager.sdk.ClientSet.BatchV1().Jobs("default").Get(manager.sdk.Ctx, "w7-surveyking-ciobztnc-build-ouctj", metav1.GetOptions{})
	// if err != nil {
	// 	t.Errorf("Error getting deployment: %v", err)
	// }
	// wrapper := NewWorkloadWrapper(deployment)
	// manager.Handle(wrapper, false)

	// 验证AppGroupApi的Persist方法是否被调用
	// 验证AppGroupApi的Persist方法是否返回错误
}

func TestEnsureWorkloadGroupNameLabel(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		workload     WorkloadWrapperInterface
		responseBody string
	}{
		{
			name: "deployment",
			path: "/apis/apps/v1/namespaces/default/deployments/demo",
			workload: NewWorkloadWrapper(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name: "demo", Namespace: "default",
			}}),
			responseBody: `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"demo","namespace":"default"}}`,
		},
		{
			name: "statefulset",
			path: "/apis/apps/v1/namespaces/default/statefulsets/demo",
			workload: NewWorkloadWrapper(&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
				Name: "demo", Namespace: "default",
			}}),
			responseBody: `{"apiVersion":"apps/v1","kind":"StatefulSet","metadata":{"name":"demo","namespace":"default"}}`,
		},
		{
			name: "daemonset",
			path: "/apis/apps/v1/namespaces/default/daemonsets/demo",
			workload: NewWorkloadWrapper(&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
				Name: "demo", Namespace: "default",
			}}),
			responseBody: `{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"demo","namespace":"default"}}`,
		},
		{
			name: "job",
			path: "/apis/batch/v1/namespaces/default/jobs/demo",
			workload: NewWorkloadWrapper(&batchv1.Job{ObjectMeta: metav1.ObjectMeta{
				Name: "demo", Namespace: "default",
			}}),
			responseBody: `{"apiVersion":"batch/v1","kind":"Job","metadata":{"name":"demo","namespace":"default"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCount := 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requestCount++
				assert.Equal(t, http.MethodPatch, r.Method)
				assert.Equal(t, tt.path, r.URL.Path)
				var patch map[string]map[string]map[string]string
				require.NoError(t, json.NewDecoder(r.Body).Decode(&patch))
				assert.Equal(t, "demo-group", patch["metadata"]["labels"][groupNameKey])
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.responseBody)),
					Request:    r,
				}, nil
			})

			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: "https://cluster.test", Transport: transport})
			require.NoError(t, err)
			manager := &WorkloadManager{sdk: &k8s.Sdk{ClientSet: clientset, Ctx: context.Background()}}

			require.NoError(t, manager.ensureWorkloadGroupNameLabel(tt.workload, "demo-group"))
			assert.Equal(t, 1, requestCount)
		})
	}
}

func TestEnsureWorkloadGroupNameLabelSkipsExistingLabel(t *testing.T) {
	manager := &WorkloadManager{}
	workload := NewWorkloadWrapper(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "demo",
		Namespace: "default",
		Labels:    map[string]string{groupNameKey: "existing-group"},
	}})

	require.NoError(t, manager.ensureWorkloadGroupNameLabel(workload, "demo-group"))
}

func TestRemoveManagedAppGroupFinalizers(t *testing.T) {
	group := &appgroupv1alpha1.AppGroup{ObjectMeta: metav1.ObjectMeta{Finalizers: []string{
		"other.example.com/finalizer",
		appGroupFinalizer,
		legacyAppGroupFinalizer,
	}}}

	require.True(t, removeManagedAppGroupFinalizers(group))
	assert.Equal(t, []string{"other.example.com/finalizer"}, group.Finalizers)
	require.False(t, removeManagedAppGroupFinalizers(group))
}

func TestIgnoreDeleteNotFound(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "apps", Resource: "deployments"}, "demo")
	require.NoError(t, ignoreDeleteNotFound(notFound))

	wantErr := context.DeadlineExceeded
	require.ErrorIs(t, ignoreDeleteNotFound(wantErr), wantErr)
}

func TestGetAppGroupFromROReturnsDeepCopy(t *testing.T) {
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	original := &appgroupv1alpha1.AppGroup{ObjectMeta: metav1.ObjectMeta{
		Name:       "demo",
		Namespace:  "default",
		Finalizers: []string{appGroupFinalizer},
	}}
	require.NoError(t, indexer.Add(original))
	manager := &WorkloadManager{AppGroupLister: appgrouplister.NewAppGroupLister(indexer)}

	group, err := manager.GetAppGroupFromRO("default", "demo")
	require.NoError(t, err)
	removeManagedAppGroupFinalizers(group)

	cached, err := manager.AppGroupLister.AppGroups("default").Get("demo")
	require.NoError(t, err)
	assert.Equal(t, []string{appGroupFinalizer}, cached.Finalizers)
}
