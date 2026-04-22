package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	K3kCvmFinalizerName = "cvm.k3k.io/finalizer"
	K3kCvmNameLabel     = "w7.cc/cvm-name"
	K3kCvmNamespaceAnno = "w7.cc/cvm-namespace"

	capacityCheckStatePending    = "pending"
	capacityCheckStateWait       = "wait"
	capacityCheckStateSuccess    = "success"
	capacityCheckStateNoResource = "no-resource"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type Cvm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CvmSpec   `json:"spec"`
	Status            CvmStatus `json:"status,omitempty"`
}

type CvmResource struct {
	CPU       int64 `json:"cpu,omitempty"`
	Memory    int64 `json:"memory,omitempty"`
	Storage   int64 `json:"storage,omitempty"`
	Bandwidth int64 `json:"bandwidth,omitempty"`
}

func (u *CvmResource) Add(rs *CvmResource) {
	if rs == nil {
		rs = &CvmResource{}
	}
	u.CPU += rs.CPU
	u.Memory += rs.Memory
	u.Storage += rs.Storage
	u.Bandwidth += rs.Bandwidth
}
func (u *CvmResource) Sub(rs *CvmResource) {
	if rs == nil {
		rs = &CvmResource{}
	}
	u.CPU -= rs.CPU
	u.Memory -= rs.Memory
	u.Storage -= rs.Storage
	u.Bandwidth -= rs.Bandwidth
}
func (u *CvmResource) IsEmpty() bool {
	return u.CPU == 0 && u.Memory == 0 && u.Storage == 0 && u.Bandwidth == 0
}

type CvmSpec struct {
	StorageClassName         string       `json:"storageClassName,omitempty"`
	Workload                 Workload     `json:"workload,omitempty"`
	UserResource             *CvmResource `json:"userResource,omitempty"`             // 强制指定资源，直接生效
	PurchasedResource        *CvmResource `json:"purchasedResource,omitempty"`        // 累计已购买资源，待容量检测后生效
	PendingPurchasedResource *CvmResource `json:"pendingPurchasedResource,omitempty"` // 购买待生效的资源
	CapacityCheckState       string       `json:"capacityCheckState,omitempty"`       // wait/no-resource/success
	BaseOrder                *CvmOrder    `json:"baseOrder,omitempty"`                // 首次购买 基础订单
	ExpandOrder              *CvmOrder    `json:"expandOrder,omitempty"`              // 扩容订单
	RenewOrder               *CvmOrder    `json:"renewOrder,omitempty"`               // 续费订单 延长到期时间
	ExpireTime               string       `json:"expireTime,omitempty"`               // 到期时间
	RecycleTime              string       `json:"recycleTime,omitempty"`              // 回收时间RECYCLE
	Rescue                   bool         `json:"rescue,omitempty"`                   // 是否救援模式

}

func (u *Cvm) AddPurchasedResource(rs *CvmResource) {
	if rs == nil {
		rs = &CvmResource{}
	}
	if u.Spec.PurchasedResource == nil {
		u.Spec.PurchasedResource = &CvmResource{}
		return
	}
	u.Spec.PurchasedResource.Add(rs)
}

// 资源检查通过
func (u *Cvm) CheckSuccess() {
	u.Spec.CapacityCheckState = capacityCheckStateSuccess
	u.AddPurchasedResource(u.Spec.PendingPurchasedResource)
}

func (u *Cvm) CheckNoResource() {
	u.Spec.CapacityCheckState = capacityCheckStateNoResource
}

func (u *Cvm) IsEmpty() bool {
	if u.Spec.PurchasedResource == nil {
		u.Spec.PurchasedResource = &CvmResource{}
	}
	if u.Spec.UserResource == nil {
		u.Spec.UserResource = &CvmResource{}
	}
	return u.Spec.UserResource.IsEmpty() && u.Spec.PurchasedResource.IsEmpty()
}

// 购买信息
type CvmOrder struct {
	OrderSn  string       `json:"orderSn"`
	Status   string       `json:"status,omitempty"`
	Resource *CvmResource `json:"resource,omitempty"`
	Hour     int          `json:"hour,omitempty"`
}

type Workload struct {
	metav1.TypeMeta `json:",inline"`
	TemplateName    string `json:"templateName"`
}

// 【微擎面板&集群云主机：云主机业务分离成独立应用】
// https://www.tapd.cn/tapd_fe/62789787/story/detail/1162789787001015242
type CvmStatus struct {
	Phase             string             `json:"phase,omitempty"`
	ReadyReplicas     int32              `json:"readyReplicas,omitempty"`
	EffectiveResource *CvmResource       `json:"effectiveResource,omitempty"` // UserResource + PurchasedResource
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CvmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Cvm `json:"items"`
}
