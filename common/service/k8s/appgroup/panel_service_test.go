package appgroup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestReconcilePanelServiceKeepsHelmService(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{
		Name: "demo", Namespace: "default", Annotations: map[string]string{"meta.helm.sh/release-name": "release"},
	}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9090}}}})
	workload := &metav1.ObjectMeta{Name: "demo", Namespace: "default", UID: types.UID("12345678-1234-1234-1234-123456789012"), Annotations: map[string]string{"meta.helm.sh/release-name": "release"}}
	template := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "demo"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Ports: []corev1.ContainerPort{{ContainerPort: 80}}}}}}
	service, err := ReconcilePanelService(ctx, client, workload, "Deployment", template)
	require.NoError(t, err)
	require.NotEqual(t, "demo", service.Name)
	require.Equal(t, int32(80), service.Spec.Ports[0].Port)
	require.Equal(t, workload.UID, service.OwnerReferences[0].UID)
	template.Spec.Containers[0].Ports[0].ContainerPort = 8080
	updated, err := ReconcilePanelService(ctx, client, workload, "Deployment", template)
	require.NoError(t, err)
	require.Equal(t, service.Name, updated.Name)
	require.Equal(t, int32(8080), updated.Spec.Ports[0].Port)
	helm, err := client.CoreV1().Services("default").Get(ctx, "demo", metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, int32(9090), helm.Spec.Ports[0].Port)
	items, err := ListAppGroupServices(ctx, client, "default", "release")
	require.NoError(t, err)
	require.Len(t, items, 2)
	template.Spec.Containers[0].Ports = nil
	_, err = ReconcilePanelService(ctx, client, workload, "Deployment", template)
	require.NoError(t, err)
	_, err = client.CoreV1().Services("default").Get(ctx, service.Name, metav1.GetOptions{})
	require.Error(t, err)
}

func TestReconcilePanelServiceRejectsUnownedName(t *testing.T) {
	workload := &metav1.ObjectMeta{Name: "demo", Namespace: "default", UID: types.UID("12345678-1234-1234-1234-123456789012")}
	name := PanelServiceName(workload.Name, string(workload.UID))
	client := fake.NewSimpleClientset(&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
	_, err := ReconcilePanelService(context.Background(), client, workload, "Deployment", corev1.PodTemplateSpec{})
	require.ErrorContains(t, err, "not owned")
}
