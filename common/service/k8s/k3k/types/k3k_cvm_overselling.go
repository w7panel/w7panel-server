package types

import (
	"fmt"

	"github.com/w7panel/w7panel/common/service/k8s/k3k/overselling"
	v1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	"k8s.io/apimachinery/pkg/api/resource"
)

type k3kCvmOverSelling struct {
	*v1alpha1.Cvm
}

func Newk3kCvmOverSelling(cvm *v1alpha1.Cvm) *k3kCvmOverSelling {
	u := &k3kCvmOverSelling{}
	u.Cvm = cvm
	return u
}

func (k *k3kCvmOverSelling) IsOverSellingWait() bool {
	return k.capacityCheckState() == "wait"
}

func (k *k3kCvmOverSelling) IsOverSellingSuccess() bool {
	return k.capacityCheckState() == "success"
}

func (k *k3kCvmOverSelling) IsOverSellingNoResource() bool {
	return k.capacityCheckState() == "no-resource"
}

func (u *k3kCvmOverSelling) NeedOverSellingCheck() bool {
	return u.CanOverSellingCheck() && !u.IsExpand()
}

// 是否可以超额检查
func (u *k3kCvmOverSelling) CanOverSellingCheck() bool {
	state := u.capacityCheckState()
	return state == "wait" || state == "no-resource"
}

func (u *k3kCvmOverSelling) GetOverResource() *overselling.Resource {
	rs := u.Spec.PendingPurchasedResource
	return &overselling.Resource{
		CPU:       resource.MustParse(fmt.Sprintf("%dm", rs.CPU)),
		Memory:    resource.MustParse(fmt.Sprintf("%dGi", rs.Memory)),
		Storage:   resource.MustParse(fmt.Sprintf("%dGi", rs.Storage)),
		BandWidth: resource.MustParse(fmt.Sprintf("%dM", rs.Bandwidth)),
	}
}

func (u *k3kCvmOverSelling) IsExpand() bool {
	return u.Spec.ExpandOrder != nil && u.Spec.ExpandOrder.Status == W7_ORDER_PAID
}

func (u *k3kCvmOverSelling) capacityCheckState() string {
	if u.Spec.CapacityCheckState != "" {
		return u.Spec.CapacityCheckState
	}
	return ""
}

// wait no-resource success
