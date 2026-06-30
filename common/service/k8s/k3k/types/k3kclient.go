package types

import (
	"context"

	microappsettingv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

func (k *K3kClient) GetK3kConfigSetting(namespace string) (*K3kConfigSetting, error) {
	setting := &microappsettingv1alpha1.MicroAppSetting{}
	err := k.k3kClient.Get(context.Background(), types.NamespacedName{Name: "default", Namespace: namespace}, setting)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return NewK3kConfig(false, "normal"), nil
		}
		return nil, err
	}
	return NewK3kConfig(setting.Spec.Login.RegistrationEnabled, "normal"), nil
}

/**
kind: MicroAppSetting
apiVersion: w7panel.w7.com/v1alpha1
metadata:
    name: default
spec:
    login:
        registrationEnabled: true

*/
