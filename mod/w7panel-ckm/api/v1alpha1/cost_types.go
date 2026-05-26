package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=costs,scope=Namespaced,shortName=cost
type Cost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CostSpec `json:"spec"`
}

type CostSpec struct {
	City         string        `json:"city,omitempty"`
	Title        string        `json:"title,omitempty"`
	PublishTitle string        `json:"publishTitle,omitempty"`
	ShowInShop   bool          `json:"showInShop,omitempty"`
	CPU          Decimal       `json:"cpu,omitempty"`
	Memory       Decimal       `json:"memory,omitempty"`
	Storage      Decimal       `json:"storage,omitempty"`
	Bandwidth    Decimal       `json:"bandwidth,omitempty"`
	Quota        *CostQuota    `json:"quota,omitempty"`
	Packages     []CostPackage `json:"packages,omitempty"`
}

type CostQuota struct {
	StorageClass string         `json:"storageClass,omitempty"`
	Hard         *CostQuotaHard `json:"hard,omitempty"`
}

type CostQuotaHard struct {
	CPU             int64  `json:"cpu,omitempty"`
	Memory          string `json:"memory,omitempty"`
	RequestsStorage int64  `json:"requestsStorage,omitempty"`
	Bandwidth       int64  `json:"bandwidth,omitempty"`
}

type CostPackage struct {
	Time              int64             `json:"time,omitempty"`
	TimeUnit          string            `json:"timeUnit,omitempty"`
	DiscountNewOpen   bool              `json:"discountNewOpen,omitempty"`
	DiscountNew       int64             `json:"discountNew,omitempty"`
	DiscountRenewOpen bool              `json:"discountRenewOpen,omitempty"`
	DiscountRenew     int64             `json:"discountRenew,omitempty"`
	Config            []CostPackageItem `json:"config,omitempty"`
}

type CostPackageItem struct {
	CPU           int64    `json:"cpu,omitempty"`
	Memory        int64    `json:"memory,omitempty"`
	Storage       int64    `json:"storage,omitempty"`
	Bandwidth     int64    `json:"bandwidth,omitempty"`
	DiscountNew   int64    `json:"discountNew,omitempty"`
	DiscountRenew int64    `json:"discountRenew,omitempty"`
	Give          bool     `json:"give,omitempty"`
	Online        bool     `json:"online,omitempty"`
	Label         string   `json:"label,omitempty"`
	Description   []string `json:"description,omitempty"`
}

// +kubebuilder:object:root=true
type CostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Cost `json:"items"`
}
