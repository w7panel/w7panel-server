package types

import (
	"context"

	"github.com/w7panel/w7panel/common/service/k8s"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type K3kClient struct {
	k3kClient client.Client
}

func NewK3kClient(client client.Client) *K3kClient {
	return &K3kClient{
		k3kClient: client,
	}
}

func (k *K3kClient) DeleteNamespace(user *K3kUser) error {
	namespace := user.GetK3kNamespace()
	// cluster :=
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	err := k.k3kClient.Delete(context.Background(), ns)
	if err != nil {
		return err
	}
	return err
}

func (k *K3kClient) GetK3kConfigSetting() (*K3kConfigSetting, error) {

	config := &configv1alpha1.K3kConfig{}
	err := k.k3kClient.Get(context.Background(), types.NamespacedName{Name: k8s.K3kConfigName}, config)
	if err != nil {
		return nil, err
	}
	secretConfig := NewK3kConfigByData(config.Spec.Data)
	return secretConfig, err
}

/**
kind: K3kConfig
apiVersion: w7panel.w7.com/v1alpha1
metadata:
    name: config
spec:
    data:
        allowConsoleRegister: "true"
        defaultPermissionName: "yibqvzoz"

*/
