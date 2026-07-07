package appgroup

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/w7panel/w7panel/common/service/k8s"
	appgroupv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOnAdd(t *testing.T) {
	sdk := k8s.NewK8sClient()
	ingress, err := sdk.ClientSet.NetworkingV1().Ingresses("default").Get(sdk.Ctx, "ing-kubhutmv1", metav1.GetOptions{})
	assert.Nil(t, err)
	sigClient, err := sdk.ToSigClient()
	assert.Nil(t, err)
	OnAddIngress(sigClient, ingress)
}

func TestOnDel(t *testing.T) {
	sdk := k8s.NewK8sClient()
	ingress, err := sdk.ClientSet.NetworkingV1().Ingresses("default").Get(sdk.Ctx, "ing-alqzvbhs", metav1.GetOptions{})
	assert.Nil(t, err)
	sigClient, err := sdk.ToSigClient()
	assert.Nil(t, err)
	OnDeleteIngress(sigClient, ingress)
}

func TestOnAddIngressSyncsDomainOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, appgroupv1alpha1.AddToScheme(scheme))
	assert.NoError(t, networkingv1.AddToScheme(scheme))

	group := &appgroupv1alpha1.AppGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: appgroupv1alpha1.SchemeGroupVersion.String(),
			Kind:       "AppGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: appgroupv1alpha1.AppGroupStatus{
			Items: []appgroupv1alpha1.AppGroupItemStatus{},
		},
	}
	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "Ingress",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-ingress",
			Namespace: "default",
			Labels: map[string]string{
				"group": "demo",
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "demo.example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/",
						}},
					},
				},
			}},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(group, ingress).Build()

	OnAddIngress(client, ingress)

	got := &appgroupv1alpha1.AppGroup{}
	assert.NoError(t, client.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, got))
	assert.Equal(t, []string{"http://demo.example.com"}, got.GetDomains())
	assert.Empty(t, got.Status.Items)
}

func TestGetResourceGroupNamesIgnoresVisibleGroups(t *testing.T) {
	obj := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"group":                "owner",
				"w7.cc/visible-groups": "site-a",
			},
		},
	}

	assert.Equal(t, []string{"owner"}, getResourceGroupNames(obj))
	assert.False(t, resourceVisibleInGroup(obj, "site-a"))
	assert.False(t, resourceVisibleInGroup(obj, "site"))
}

func TestGetResourceGroupNamesSupportsHelmMetadata(t *testing.T) {
	obj := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app.kubernetes.io/instance": "helm-instance",
			},
			Annotations: map[string]string{
				"meta.helm.sh/release-name": "helm-release",
			},
		},
	}

	assert.Equal(t, []string{"helm-instance", "helm-release"}, getResourceGroupNames(obj))
	assert.True(t, resourceVisibleInGroup(obj, "helm-release"))
}
