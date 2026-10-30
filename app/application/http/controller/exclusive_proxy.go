package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// checkExclusiveProxyWrite validates the final CRD, including partial updates, before Kubernetes receives it.
func checkExclusiveProxyWrite(ctx *gin.Context, path string) error {
	if ctx.Request.Method != http.MethodPost && ctx.Request.Method != http.MethodPut && ctx.Request.Method != http.MethodPatch && ctx.Request.Method != http.MethodDelete {
		return nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "namespaces" && parts[4] == "configmaps" {
		protected := len(parts) >= 6 && parts[5] == permissionservice.ExclusiveConfigMapName
		if len(parts) == 5 && ctx.Request.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 2<<20+1))
			if err != nil || len(body) > 2<<20 {
				return fmt.Errorf("ConfigMap 请求体无效或过大")
			}
			ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
			var object struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(body, &object); err != nil {
				return err
			}
			protected = object.Metadata.Name == permissionservice.ExclusiveConfigMapName
		}
		if protected {
			return fmt.Errorf("专属权限策略仅允许集群管理员在面板外维护")
		}
	}
	if ctx.Request.Method == http.MethodDelete || len(parts) < 4 || parts[0] != "apis" || parts[1] != "w7panel.w7.com" || parts[2] != "v1alpha1" || parts[3] != "permissions" && parts[3] != "users" {
		return nil
	}
	if ctx.Request.Method == http.MethodPost && len(parts) != 4 || ctx.Request.Method != http.MethodPost && len(parts) != 5 {
		return fmt.Errorf("不支持的权限资源写入路径")
	}
	sdk := k8s.NewK8sClient().Sdk
	policy, err := permissionservice.LoadExclusivePolicy(ctx.Request.Context(), sdk)
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 2<<20+1))
	if err != nil || len(body) > 2<<20 {
		return fmt.Errorf("权限请求体无效或过大")
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	if ctx.Request.Method == http.MethodPatch {
		body, err = patchedExclusiveObject(ctx.Request.Context(), sdk, parts[3], parts[4], ctx.GetHeader("Content-Type"), body)
		if err != nil {
			return err
		}
	}
	var object struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Role            string                             `json:"role"`
			UserMode        string                             `json:"userMode"`
			PermissionName  string                             `json:"permissionName"`
			MenuRules       []string                           `json:"menuRules"`
			APIRules        []configv1alpha1.PermissionAPIRule `json:"apiRules"`
			DomainWhiteList []configv1alpha1.DomainWhiteItem   `json:"domainWhiteList"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &object); err != nil {
		return err
	}
	clusterType := permissionservice.CurrentClusterType()
	if err := policy.ValidateGrant(clusterType, object.Spec.MenuRules, object.Spec.APIRules); err != nil {
		return err
	}
	if clusterType == permissionservice.ChildClusterType && parts[3] == "permissions" {
		if ctx.Request.Method == http.MethodPost {
			if len(object.Spec.DomainWhiteList) != 0 {
				return fmt.Errorf("子集群域名白名单由主集群统一管理")
			}
		} else {
			current, err := permissionservice.Get(ctx.Request.Context(), sdk, parts[4])
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(current.Spec.DomainWhiteList, object.Spec.DomainWhiteList) && !(len(current.Spec.DomainWhiteList) == 0 && len(object.Spec.DomainWhiteList) == 0) {
				return fmt.Errorf("子集群域名白名单由主集群统一管理")
			}
		}
	}
	if parts[3] == "users" && object.Spec.PermissionName != "" {
		permission, err := permissionservice.Get(ctx.Request.Context(), sdk, permissionservice.NormalizePermissionName(object.Spec.PermissionName))
		if err != nil {
			return err
		}
		return policy.ValidateGrant(clusterType, permission.Spec.MenuRules, permission.Spec.APIRules)
	}
	return nil
}

func patchedExclusiveObject(ctx context.Context, sdk *k8s.Sdk, resource, name, contentType string, patch []byte) ([]byte, error) {
	current, err := sdk.DynamicClient().Resource(schema.GroupVersionResource{Group: "w7panel.w7.com", Version: "v1alpha1", Resource: resource}).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	old, err := json.Marshal(current.Object)
	if err != nil {
		return nil, err
	}
	switch strings.Split(contentType, ";")[0] {
	case "application/merge-patch+json":
		return jsonpatch.MergePatch(old, patch)
	case "application/json-patch+json":
		decoded, err := jsonpatch.DecodePatch(patch)
		if err != nil {
			return nil, err
		}
		return decoded.Apply(old)
	default:
		return nil, fmt.Errorf("不支持的权限补丁类型")
	}
}
