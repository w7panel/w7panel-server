package appgroup

import (
	"github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateAppGroup(name string, namespace string) *v1alpha1.AppGroup {
	// 创建 AppGroup 对象
	appGroup := &v1alpha1.AppGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AppGroup",
			APIVersion: "w7panel.w7.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{"w7panel.w7.com/finalizer"},
		},
	}
	return appGroup
}
