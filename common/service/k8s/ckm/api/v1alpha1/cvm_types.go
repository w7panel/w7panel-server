package v1alpha1

import (
	"strings"
	"time"

	"github.com/aws/smithy-go/ptr"
	k3kv1 "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/shopspring/decimal"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// 【微擎面板&集群云主机：云主机业务分离成独立应用】
// https://www.tapd.cn/tapd_fe/62789787/story/detail/1162789787001015242
const (
	K3kCkmFinalizerName = "ckm.k3k.io/finalizer"
	K3kCkmNameLabel     = "w7.cc/ckm-name"
	K3kCkmNamespaceAnno = "w7.cc/ckm-namespace"
	CkmExpired          = "w7.cc/expired"
	LabelPhase          = "w7.cc/phase"

	capacityCheckStatePending    = "pending"
	capacityCheckStateWait       = "wait"
	capacityCheckStateSuccess    = "success"
	capacityCheckStateNoResource = "no-resource"

	BASE_BUY   = "base"   // 基础购买
	RENEW_BUY  = "renew"  // 续费购买
	EXPAND_BUY = "expand" // 扩容购买
)

type Phase string

const (
	// ClusterPending      = ClusterPhase("Pending") //创建中
	// ClusterProvisioning = ClusterPhase("Provisioning") //配置中
	// ClusterReady        = ClusterPhase("Ready") //运行中
	// ClusterFailed       = ClusterPhase("Failed") //失败
	// ClusterTerminating  = ClusterPhase("Terminating") //回收中
	// ClusterUnknown      = ClusterPhase("Unknown") //未知
	PhaseNew        = Phase("new")        //无资源
	PhaseReady      = Phase("ready")      //有资源
	PhaseRecycle    = Phase("recycle")    //待回收
	PhaseRecycleing = Phase("recycleing") //回收中
	PhaseCreating   = Phase("creating")   //创建中

	ClusterStopped = k3kv1.ClusterPhase("stopped") //暂停中
)

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ckms,scope=Namespaced,shortName=ckm
// +kubebuilder:subresource:status
type Ckm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CkmSpec   `json:"spec"`
	Status            CkmStatus `json:"status,omitempty"`
}

type CkmSpec struct {
	StorageClassName         string               `json:"storageClassName,omitempty"`
	CostName                 string               `json:"costName,omitempty"`
	PermissionName           string               `json:"permissionName,omitempty"`
	Workload                 Workload             `json:"workload,omitempty"`
	UserResource             *CkmResource         `json:"userResource,omitempty"`             // 强制指定资源，直接生效
	PurchasedResource        *CkmResource         `json:"purchasedResource,omitempty"`        // 累计已购买资源，待容量检测后生效
	PendingPurchasedResource *CkmResource         `json:"pendingPurchasedResource,omitempty"` // 购买待生效的资源
	CapacityCheckState       string               `json:"capacityCheckState,omitempty"`       // wait/no-resource/success
	BaseOrder                *CkmOrder            `json:"baseOrder,omitempty"`                // 首次购买 基础订单
	ExpandOrder              *CkmOrder            `json:"expandOrder,omitempty"`              // 扩容订单
	RenewOrder               *CkmOrder            `json:"renewOrder,omitempty"`               // 续费订单 延长到期时间
	ReturnOrders             map[string]*CkmOrder `json:"returnOrders,omitempty"`             // 退款订单
	ExpireTime               string               `json:"expireTime,omitempty"`               // 到期时间
	RecycleTime              string               `json:"recycleTime,omitempty"`              // 回收时间RECYCLE
	Rescue                   bool                 `json:"rescue,omitempty"`                   // 是否救援模式
	Pause                    bool                 `json:"pause,omitempty"`                    // 暂停 (到期或暂停 会删除k3k cluster)

}

func (u *Ckm) GetAgentName() string {
	return GetK3kAgentName(u.GetK3kName())
}

func (u *Ckm) GetK3kName() string {
	return strings.ReplaceAll(u.Namespace, "k3k-", "")
}
func (u *Ckm) GetVirtualIngressServiceName() string {
	return GetVirtualIngressServiceName(u.Namespace, u.Name)
}

func (u *Ckm) GetK3kNamespace() string {
	return u.Namespace
}
func (u *Ckm) AddPurchasedResource(rs *CkmResource) {
	if rs == nil {
		rs = &CkmResource{}
	}
	if u.Spec.PurchasedResource == nil {
		u.Spec.PurchasedResource = &CkmResource{}
	}
	u.Spec.PurchasedResource.Add(rs)
}

// 是否需要判断资源是否充足
func (u *Ckm) CanOverSellingCheck() bool {
	return u.Spec.CapacityCheckState == capacityCheckStateNoResource || u.Spec.CapacityCheckState == capacityCheckStateWait
}

// 资源检查通过
func (u *Ckm) CheckSuccess() {
	u.Spec.CapacityCheckState = capacityCheckStateSuccess
	u.AddPurchasedResource(u.Spec.PendingPurchasedResource)
	u.Spec.PendingPurchasedResource = &CkmResource{}
}

func (u *Ckm) CheckNoResource() {
	u.Spec.CapacityCheckState = capacityCheckStateNoResource
}

func (u *Ckm) IsEmpty() bool {
	if u.Spec.PurchasedResource == nil {
		u.Spec.PurchasedResource = &CkmResource{}
	}
	if u.Spec.UserResource == nil {
		u.Spec.UserResource = &CkmResource{}
	}
	return u.Spec.UserResource.IsEmpty() && u.Spec.PurchasedResource.IsEmpty()
}

func (u *Ckm) IsPendingEmpty() bool {
	if u.Spec.PendingPurchasedResource == nil {
		u.Spec.PendingPurchasedResource = &CkmResource{}
	}

	return u.Spec.PendingPurchasedResource.IsEmpty()
}

func (u *Ckm) GetClusterServer0PvcName() string {
	return "varlibrancherk3s-k3k-" + u.Name + "-server-0"
}

// 购买信息
type CkmOrder struct {
	OrderSn      string       `json:"orderSn"`
	Status       string       `json:"status,omitempty"`
	Resource     *CkmResource `json:"resource,omitempty"`
	Hour         int          `json:"hour,omitempty"`
	BuyMode      string       `json:"buyMode,omitempty"`
	ReturnFinish bool         `json:"returnFinish,omitempty"` //退款是否处理结束
}

type Workload struct {
	APIVersion   string `json:"apiVersion,omitempty"`
	Kind         string `json:"kind,omitempty"`
	TemplateName string `json:"templateName"`
	Token        string `json:"token"` //用于创建k3s集群的token
}

type CkmStatus struct {
	Phase                string             `json:"phase,omitempty"`
	ClusterPhase         k3kv1.ClusterPhase `json:"clusterPhase,omitempty"`
	EffectiveResource    *CkmResource       `json:"effectiveResource,omitempty"` // UserResource + PurchasedResource
	Conditions           []metav1.Condition `json:"conditions,omitempty"`
	IsExpired            *bool              `json:"isExpired,omitempty"`    //是否过期
	IsRecycling          *bool              `json:"isRecycling,omitempty"`  //是否回收中
	CanBaseBuy           *bool              `json:"canBaseBuy,omitempty"`   //是否可以购买基础套餐
	CanExpandBuy         *bool              `json:"canExpandBuy,omitempty"` //是否可以扩容
	CanRenewBuy          *bool              `json:"canRenewBuy,omitempty"`  //是否可以续费
	CanDelete            *bool              `json:"canDelete,omitempty"`    //是否可以续费
	DiffMonth            string             `json:"diffMonth,omitempty"`    // 到期时间剩余月数
	Server0PodName       string             `json:"server0PodName,omitempty"`
	Server0ContainerName string             `json:"server0ContainerName,omitempty"`
	K3kStatufulSetName   string             `json:"k3kStatufulSetName,omitempty"`
	RescueJobName        string             `json:"rescueJobName,omitempty"` //救援job名称
	RescuePhase          string             `json:"rescuePhase,omitempty"`   //救援job状态 running failed success
}

// 进入退出救援模式
func (u *Ckm) RescueToggle() {
	u.Spec.Rescue = !u.Spec.Rescue //进入退出救援模式
}
func (u *Ckm) GetRescueJobName() string {
	return "k3k-" + u.Name + "-rescue"
}

func (u *Ckm) GetK3kSecretTokenName() string {
	return "k3k-" + u.Name + "-token"
}
func (u *Ckm) computeDefault() {
	u.Status.Server0PodName = "k3k-" + u.Name + "-server-0"
	u.Status.Server0ContainerName = "k3k-" + u.Name + "-server"
	u.Status.K3kStatufulSetName = "k3k-" + u.Name + "-server"
	u.Status.RescueJobName = u.GetRescueJobName()
	//是否可以删除
}
func (u *Ckm) computeBuy() {
	u.Status.CanBaseBuy = ptr.Bool(u.IsEmpty() && u.IsPendingEmpty())         //是否购买基础套餐
	u.Status.CanExpandBuy = ptr.Bool(!u.IsEmpty() && u.Spec.ExpireTime != "") //是否可以扩容
	u.Status.CanRenewBuy = ptr.Bool(!u.IsEmpty() && u.Spec.ExpireTime != "")  //是否可以续费
	u.Status.CanDelete = ptr.Bool(u.IsEmpty())                                //是否可以删除
}
func (u *Ckm) ComputeStatus() {
	if u.Labels == nil {
		u.Labels = map[string]string{}
	}

	isExpired := false
	isRecycling := false
	u.Status.IsExpired = &isExpired
	u.Status.IsRecycling = &isRecycling
	if u.Spec.ExpireTime != "" {
		etime, err := time.Parse(time.DateTime, u.Spec.ExpireTime)
		if err == nil {
			*u.Status.IsExpired = etime.Before(time.Now())
			u.Labels[CkmExpired] = u.Name
			if u.Spec.RecycleTime == "" {
				u.Spec.RecycleTime = etime.Add(time.Hour * 24 * 3).Format(time.DateTime)
			}
			diffMonth := u.getDiffMonths(etime)
			u.Status.DiffMonth = diffMonth.String()
		}
	}
	if u.Spec.RecycleTime != "" {
		rtime, err := time.Parse(time.DateTime, u.Spec.RecycleTime)
		if err == nil {
			if rtime.Before(time.Now()) {
				*u.Status.IsRecycling = true
			}
		}
	}

	// 无资源 有资源 待回收 回收中 创建
	if u.IsEmpty() {
		u.Status.Phase = string(PhaseNew)
	} else {
		if u.Status.IsExpired != nil && *u.Status.IsExpired {
			u.Status.Phase = string(PhaseRecycle)
			u.Status.ClusterPhase = ClusterStopped
			if u.Status.IsRecycling != nil && *u.Status.IsRecycling {
				u.Status.Phase = string(PhaseNew) //超过回收时间 则清理成无资源
			}
		} else {
			u.Status.Phase = string(PhaseCreating)
			if u.Status.ClusterPhase == k3kv1.ClusterTerminating {
				u.Status.Phase = string(PhaseRecycleing) //回收中
			}
			if u.Status.ClusterPhase == k3kv1.ClusterReady {
				u.Status.Phase = string(PhaseReady) //有资源
			}
		}
	}
	//判断是否回收中
	u.Status.EffectiveResource = &CkmResource{}
	u.Status.EffectiveResource.Add(u.Spec.UserResource)
	u.Status.EffectiveResource.Add(u.Spec.PurchasedResource)
	u.computeBuy()
	u.computeDefault()
}

func (u *Ckm) getDiffMonths(expireTime time.Time) decimal.Decimal {
	if expireTime.Before(time.Now()) {
		return decimal.Zero
	}
	hours := expireTime.Sub(time.Now()).Hours()
	diffMonths := decimal.NewFromFloat(hours).Div(decimal.NewFromFloat(24 * 30))
	return (diffMonths)
}

func (u *Ckm) HasReturnNoProcessOrder() bool {

	if u.Spec.ReturnOrders == nil {
		return false
	}
	for _, order := range u.Spec.ReturnOrders {
		if !order.ReturnFinish {
			return true
		}
	}
	return false
}

func (u *Ckm) GetReturnNoProcessOrder() *CkmOrder {

	if u.Spec.ReturnOrders == nil {
		return nil
	}
	for _, order := range u.Spec.ReturnOrders {
		if !order.ReturnFinish {
			return order
		}
	}
	return nil
}

func (u *Ckm) ProcessReturnOrder(orderSn string) error {
	if u.Spec.ReturnOrders == nil {
		return nil
	}
	for _, order := range u.Spec.ReturnOrders {
		if !order.ReturnFinish && order.OrderSn == orderSn {
			err := u.doReturn(order)
			if err != nil {
				return err
			}
			order.ReturnFinish = true
			return nil
		}
	}
	return nil
}

func (u *Ckm) doReturn(order *CkmOrder) error {
	if order.BuyMode == BASE_BUY {
		u.Spec.PurchasedResource.Sub(order.Resource)
		u.Spec.ExpireTime = time.Now().Format(time.DateTime)
	}
	if order.BuyMode == EXPAND_BUY {
		u.Spec.PurchasedResource.Sub(order.Resource)
	}
	if order.BuyMode == RENEW_BUY {

		expireTime, err := time.Parse(time.DateTime, u.Spec.ExpireTime)
		if err != nil {
			return err
		}
		// 当前时间减去hour 小时
		expireTime = expireTime.Add(-time.Hour * time.Duration(int64(order.Hour)))
		u.Spec.ExpireTime = expireTime.Format(time.DateTime)
	}
	return nil
}

// +kubebuilder:object:root=true
type CkmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Ckm `json:"items"`
}

type CkmResource struct {
	CPU       int64 `json:"cpu,omitempty"`
	Memory    int64 `json:"memory,omitempty"`
	Storage   int64 `json:"storage,omitempty"`
	Bandwidth int64 `json:"bandwidth,omitempty"`
}

func (u *CkmResource) Add(rs *CkmResource) {
	if rs == nil {
		rs = &CkmResource{}
	}
	u.CPU += rs.CPU
	u.Memory += rs.Memory
	u.Storage += rs.Storage
	u.Bandwidth += rs.Bandwidth
}
func (u *CkmResource) Sub(rs *CkmResource) {
	if rs == nil {
		rs = &CkmResource{}
	}
	u.CPU -= rs.CPU
	u.Memory -= rs.Memory
	u.Storage -= rs.Storage
	u.Bandwidth -= rs.Bandwidth
}
func (u *CkmResource) IsEmpty() bool {
	return u.CPU == 0 && u.Memory == 0 && u.Storage == 0 && u.Bandwidth == 0
}
