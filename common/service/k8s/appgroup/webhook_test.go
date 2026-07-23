package appgroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetResourceGroupNamesSupportsGroupNames(t *testing.T) {
	obj := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"group":             "owner",
				"w7.cc/group-names": "site-a",
			},
		},
	}

	assert.Equal(t, []string{"owner", "site-a"}, getResourceGroupNames(obj))
	assert.True(t, resourceVisibleInGroup(obj, "site-a"))
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
