package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const ExclusiveMenusConfigMap = "w7panel-exclusive-menus"

type ExclusiveMenuRule struct {
	MenuRule     string   `json:"menuRule"`
	ClusterTypes []string `json:"clusterTypes"`
}

type ExclusiveMenus struct {
	Rules           []ExclusiveMenuRule `json:"rules"`
	ResourceVersion string              `json:"resourceVersion"`
}

func (p ExclusiveMenus) Validate() error {
	if p.Rules == nil {
		return fmt.Errorf("缺少菜单规则")
	}
	seen := map[string]bool{}
	for _, rule := range p.Rules {
		path := rule.MenuRule
		if path == "" || strings.Trim(path, "/ ") != path || strings.Contains(path, "//") || strings.ContainsAny(path, " \t\r\n*?#") || seen[path] {
			return fmt.Errorf("无效或重复的菜单路径 %q", path)
		}
		seen[path] = true
		if len(rule.ClusterTypes) == 0 || len(rule.ClusterTypes) > 2 {
			return fmt.Errorf("菜单 %q 缺少集群范围", path)
		}
		allowed := map[string]bool{}
		for _, clusterType := range rule.ClusterTypes {
			if (clusterType != "main" && clusterType != "child") || allowed[clusterType] {
				return fmt.Errorf("菜单 %q 的集群范围无效", path)
			}
			allowed[clusterType] = true
		}
	}
	for _, child := range p.Rules {
		for _, parent := range p.Rules {
			if !strings.HasPrefix(child.MenuRule, parent.MenuRule+"/") {
				continue
			}
			for _, clusterType := range child.ClusterTypes {
				if !containsClusterType(parent.ClusterTypes, clusterType) {
					return fmt.Errorf("菜单 %q 超出父菜单 %q 的集群范围", child.MenuRule, parent.MenuRule)
				}
			}
		}
	}
	return nil
}

func containsClusterType(types []string, target string) bool {
	for _, value := range types {
		if value == target {
			return true
		}
	}
	return false
}

func LoadExclusiveMenus(ctx context.Context, sdk *k8s.Sdk) (ExclusiveMenus, error) {
	if helper.IsChildAgent() {
		var policy ExclusiveMenus
		if err := loadMainSync(ctx, "/exclusive-menus", &policy); err != nil {
			return policy, err
		}
		return policy, policy.Validate()
	}
	cm, err := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace()).Get(ctx, ExclusiveMenusConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ExclusiveMenus{Rules: []ExclusiveMenuRule{}}, nil
	}
	if err != nil {
		return ExclusiveMenus{}, err
	}
	var policy ExclusiveMenus
	if err := json.Unmarshal([]byte(cm.Data["policy"]), &policy); err != nil {
		return policy, err
	}
	policy.ResourceVersion = cm.ResourceVersion
	return policy, policy.Validate()
}

func SaveExclusiveMenus(ctx context.Context, sdk *k8s.Sdk, policy ExclusiveMenus) (ExclusiveMenus, error) {
	if err := policy.Validate(); err != nil {
		return ExclusiveMenus{}, err
	}
	data, err := json.Marshal(ExclusiveMenus{Rules: policy.Rules})
	if err != nil {
		return ExclusiveMenus{}, err
	}
	client := sdk.ClientSet.CoreV1().ConfigMaps(sdk.GetNamespace())
	cm, err := client.Get(ctx, ExclusiveMenusConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if policy.ResourceVersion != "" {
			return ExclusiveMenus{}, apierrors.NewConflict(corev1.Resource("configmaps"), ExclusiveMenusConfigMap, fmt.Errorf("规则已变更"))
		}
		cm, err = client.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: ExclusiveMenusConfigMap}, Data: map[string]string{"policy": string(data)}}, metav1.CreateOptions{})
	} else if err == nil {
		if policy.ResourceVersion == "" || policy.ResourceVersion != cm.ResourceVersion {
			return ExclusiveMenus{}, apierrors.NewConflict(corev1.Resource("configmaps"), ExclusiveMenusConfigMap, fmt.Errorf("规则已变更"))
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data["policy"] = string(data)
		cm, err = client.Update(ctx, cm, metav1.UpdateOptions{})
	}
	if err != nil {
		return ExclusiveMenus{}, err
	}
	policy.ResourceVersion = cm.ResourceVersion
	return policy, nil
}
