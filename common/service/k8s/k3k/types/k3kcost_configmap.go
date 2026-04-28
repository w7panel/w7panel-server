package types

import (
	"errors"
	"log/slog"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

type param map[string]string
type Params []param

type K3kCostConfigMap struct {
	*corev1.ConfigMap
	cost      *K3kCost
	limitOnce sync.Once
	onceCost  sync.Once
}

func NewK3kCostConfigMap(v *corev1.ConfigMap) *K3kCostConfigMap {
	return &K3kCostConfigMap{
		ConfigMap: v,
	}
}

func (v *K3kCostConfigMap) GetCost() *K3kCost {
	// 返回K3kGroup的名称
	v.onceCost.Do(func() {
		v.cost = v.getCost()
	})
	return v.cost
}

func (v *K3kCostConfigMap) getCost() *K3kCost {

	cost, err := ConfigMapToCost(v.ConfigMap)
	if err != nil {
		slog.Error("parse cost config error", "error", err)
		return nil
	}
	return cost
}

func (v *K3kCostConfigMap) getLimitRange() *LimitRangeQuota {
	jstr, ok := v.Annotations[W7_QUOTA_LIMIT]
	if ok {
		lqr2, err := NewLimitRangeQuata(jstr)
		if err != nil {
			slog.Error("parse quota limit error", "error", err)
			return nil
		}
		return lqr2
	}
	return nil
}

func (v *K3kCostConfigMap) CanPublish() bool {
	return v.cost != nil
}

func (u *K3kCostConfigMap) GetOrderCompute() (*K3kOrderCompute, error) {
	if u.getCost() == nil {
		return nil, errors.New("当前用户未配置费用套餐，无法购买")
	}

	return NewK3kOrderComputeWithCost(u.getCost()), nil
}

func (b *K3kCostConfigMap) ToPublishShopParams2(name string) (map[string]interface{}, error) {
	items, err := b.ToPackageItemsParams(true)
	if err != nil {
		return nil, err
	}
	title := b.Annotations["title"]
	pubTitle, ok := b.Annotations["publish-title"]
	if ok {
		title = pubTitle
	}

	return map[string]interface{}{
		"items":     items,
		"groupname": name,
		"title":     title,
		"city":      b.Annotations["city"],
	}, nil
}

func (b *K3kCostConfigMap) ToPackageItemsParams(onlyOnline bool) (Params, error) {

	params := Params{}
	city := b.Annotations["city"]
	title := b.Annotations["title"]
	pubTitle, ok := b.Annotations["publish-title"]
	demo := "1" // 默认非演示环境
	if (b.Labels != nil) && (b.Labels["w7.cc/demo-user"] == "true") {
		demo = "2" // 演示环境
	}
	if ok {
		title = pubTitle
	}
	if b.GetCost() != nil {
		compute, err := b.GetOrderCompute()
		if err != nil {
			return nil, err
		}

		packages := b.GetCost().Packages
		for _, pkg := range packages {
			discountNew := pkg.DiscountNew
			if discountNew.value <= 0 {
				discountNew.value = 100
			}
			items := pkg.Items
			if onlyOnline {
				items = pkg.OnLineItems()
			}
			for _, item := range items {
				buymode := "buy"
				if item.IsGive {
					buymode = "give"
				}
				itemDiscountNew := item.DiscountNew
				if itemDiscountNew.value <= 0 {
					itemDiscountNew.value = 100
				}
				if itemDiscountNew.Value() == 100 || itemDiscountNew.Value() == 0 {
					itemDiscountNew = discountNew
				}
				param := param{
					"cpu":         item.Cpu.String(),
					"memory":      item.Memory.String(),
					"storage":     item.Storage.String(),
					"bandwidth":   item.Bandwidth.String(),
					"discountnew": itemDiscountNew.String(),
					// "discountNew": item.DiscountNew.String(),
					// "discountRenew": item.DiscountRenew.String(),
					"groupname":   b.Name,
					"city":        city,
					"title":       title,
					"buymode":     buymode,
					"quantity":    pkg.Quantity.String(),
					"unit":        pkg.Unit,
					"label":       item.Label,
					"description": strings.Join(item.Description, "|"),
					"demo":        demo,
				}
				computeItem := compute.WithResource(item.ToBuyResource()).WithQuantity(pkg.ToUnitQuantity())
				param["price"] = computeItem.GetOriginPrice().String()
				param["discountprice"] = computeItem.GetDiscountPriceNotGive("base").String()
				params = append(params, param)
			}
		}
	}
	return params, nil
}
