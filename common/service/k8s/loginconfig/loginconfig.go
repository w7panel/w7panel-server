package loginconfig

import (
	"context"

	"github.com/w7panel/w7panel/common/service/k8s"
	loginconfigv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/loginconfig/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const DefaultName = "default"

func Get(ctx context.Context, sdk *k8s.Sdk) (*loginconfigv1alpha1.LoginConfig, error) {
	config := &loginconfigv1alpha1.LoginConfig{}
	client, err := sdk.ToSigClient()
	if err != nil {
		return config, err
	}
	err = client.Get(ctx, types.NamespacedName{Name: DefaultName}, config)
	return config, err
}

func Provider(config *loginconfigv1alpha1.LoginConfig, providerType string) (loginconfigv1alpha1.LoginProvider, bool) {
	if config == nil {
		return loginconfigv1alpha1.LoginProvider{}, false
	}
	for _, provider := range config.Spec.Providers {
		if provider.Type == providerType {
			return provider, true
		}
	}
	return loginconfigv1alpha1.LoginProvider{}, false
}

// Default creates the immutable built-in provider. It is deliberately only
// created when absent by the upgrade script, never applied over user settings.
func Default() *loginconfigv1alpha1.LoginConfig {
	return &loginconfigv1alpha1.LoginConfig{
		TypeMeta:   metav1.TypeMeta{APIVersion: "w7panel.w7.com/v1alpha1", Kind: "LoginConfig"},
		ObjectMeta: metav1.ObjectMeta{Name: DefaultName},
		Spec: loginconfigv1alpha1.LoginConfigSpec{Providers: []loginconfigv1alpha1.LoginProvider{
			{Type: "we7-cloud", Enabled: true, Immutable: true},
			{Type: "oidc", Enabled: false, Scopes: []string{"openid", "profile"}},
		}},
	}
}
