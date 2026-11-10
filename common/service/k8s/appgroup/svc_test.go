package appgroup

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/w7panel/w7panel/common/service/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestCreateSvcAnnotationDoesNotCreateService(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status","reason":"NotFound","code":404}`)), Request: r}, nil
	})
	client, err := kubernetes.NewForConfig(&rest.Config{Host: "https://cluster.test", Transport: transport, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
	require.NoError(t, err)
	manager := &WorkloadManager{sdk: &k8s.Sdk{ClientSet: client, Ctx: context.Background()}}
	workload := NewWorkloadWrapper(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default", Annotations: map[string]string{"w7.cc/create-svc": "true"}},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Ports: []corev1.ContainerPort{{ContainerPort: 80}},
		}}}}},
	})
	require.NoError(t, manager.fixSvc(workload, false))
}
