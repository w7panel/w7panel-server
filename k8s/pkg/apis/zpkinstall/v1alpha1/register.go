package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var SchemeGroupVersion = schema.GroupVersion{Group: "w7panel.w7.com", Version: "v1alpha1"}
var SchemeBuilder = runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion, &ZpkInstall{}, &ZpkInstallList{})
	metav1.AddToGroupVersion(s, SchemeGroupVersion)
	return nil
})
var AddToScheme = SchemeBuilder.AddToScheme
