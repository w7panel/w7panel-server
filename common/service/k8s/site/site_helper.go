package site

import (
	"context"

	"github.com/w7panel/w7panel/common/service/k8s"
	microappsettingv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

const globalMicroAppSettingName = "default"

func GetGlobalMicroAppSetting(ctx context.Context, sdk *k8s.Sdk) (*microappsettingv1alpha1.MicroAppSetting, error) {
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	setting := &microappsettingv1alpha1.MicroAppSetting{}
	err = sigClient.Get(ctx, types.NamespacedName{
		Name:      globalMicroAppSettingName,
		Namespace: sdk.GetNamespace(),
	}, setting)
	if err != nil {
		return nil, err
	}
	return setting, nil
}
