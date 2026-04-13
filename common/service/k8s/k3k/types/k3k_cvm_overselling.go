package types

import (
	"github.com/w7panel/w7panel/common/service/k8s/k3k/overselling"
	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
)

type k3kCvmOverSelling struct {
	*v1alpha1.Cvm
	*k3kUserBase
	overResource     *overselling.Resource
	overBaseResource *overselling.Resource
}

func Newk3kCvmOverSelling(cvm *v1alpha1.Cvm) *k3kCvmOverSelling {
	u := &k3kCvmOverSelling{}
	u.Cvm = cvm
	u.overResource = overselling.EmptyResource()
	overstr, ok2 := cvm.Annotations[W7_OVER_RESOURCE]
	if ok2 {
		u.overResource = overselling.CreateFromString(overstr)
	}
	u.overBaseResource = overselling.EmptyResource()
	overBasestr, ok3 := cvm.Annotations[W7_OVER_BASE_RESOURCE]
	if ok3 {
		u.overBaseResource = overselling.CreateFromString(overBasestr)
	}
	return u
}

func (k *k3kCvmOverSelling) IsOverSellingWait() bool {
	return k.Labels[W7_OVER_MODE] == "wait"
}

func (k *k3kCvmOverSelling) IsOverSellingSuccess() bool {
	return k.Labels[W7_OVER_MODE] == "success"
}

func (k *k3kCvmOverSelling) IsOverSellingNoResource() bool {
	return k.Labels[W7_OVER_MODE] == "no-resource"
}

func (u *k3kCvmOverSelling) NeedOverSellingCheck() bool {
	return u.CanOverSellingCheck() && !u.IsExpand()
}

// 是否可以超额检查
func (u *k3kCvmOverSelling) CanOverSellingCheck() bool {
	return u.Labels[W7_OVER_MODE] == "wait" || u.Labels[W7_OVER_MODE] == "no-resource"
}

func (u *k3kCvmOverSelling) GetOverResource() *overselling.Resource {
	if u.IsExpand() {
		return u.overResource
	}
	return u.overBaseResource
}

func (u *k3kCvmOverSelling) IsExpand() bool {
	return u.Labels[W7_EXPAND_ORDER_STATUS] == W7_ORDER_PAID
}

// wait no-resource success
