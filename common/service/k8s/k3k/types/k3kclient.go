package types

import (
	"context"

	"github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"

	// _ "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"

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

func (k *K3kClient) Delete(user *K3kUser) error {
	namespace := user.GetK3kNamespace()
	clusterName := user.GetK3kName()
	// cluster :=
	cluster := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "k3k.io/v1alpha1",
		},
	}

	err := k.k3kClient.Delete(context.Background(), cluster)
	if err != nil {
		return err
	}
	return err
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

func (k *K3kClient) GetPolicy(user *K3kUser) (*v1alpha1.VirtualClusterPolicy, error) {
	policy := &v1alpha1.VirtualClusterPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.GetClusterPolicy(),
			Namespace: user.GetK3kNamespace(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualClusterPolicy",
			APIVersion: "k3k.io/v1alpha1",
		},
	}
	err := k.k3kClient.Get(context.Background(), types.NamespacedName{Namespace: user.Namespace, Name: user.GetClusterPolicy()}, policy)
	if err != nil {
		return nil, err
	}
	return policy, err
}

func (k *K3kClient) GetPolicyByName(name string) (*v1alpha1.VirtualClusterPolicy, error) {
	policy := &v1alpha1.VirtualClusterPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualClusterPolicy",
			APIVersion: "k3k.io/v1alpha1",
		},
	}
	err := k.k3kClient.Get(context.Background(), types.NamespacedName{Name: name}, policy)
	if err != nil {
		return nil, err
	}
	return policy, err
}

func (k *K3kClient) GetCluster(user *K3kUser) (*v1alpha1.Cluster, error) {
	cluster := &v1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      user.GetK3kName(),
			Namespace: user.GetK3kNamespace(),
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: "k3k.io/v1alpha1",
		},
	}
	err := k.k3kClient.Get(context.Background(), types.NamespacedName{Namespace: user.Namespace, Name: user.GetK3kName()}, cluster)
	if err != nil {
		return nil, err
	}
	return cluster, err
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
    name: k3k.config
spec:
    data:
        allowConsoleRegister: "true"
        defaultPermissionName: "yibqvzoz"

*/
