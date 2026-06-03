// +kubebuilder:object:generate=true
// +groupName=ckm.w7.cc
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "ckm.w7.cc", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&Ckm{}, &CkmList{}, &CkmConsoleOrder{}, &CkmConsoleOrderList{}, &Cost{}, &CostList{})
}
