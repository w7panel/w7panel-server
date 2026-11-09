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
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Child permissions keep their domain whitelist under main-cluster control.
func checkChildWhitelistProxyWrite(ctx *gin.Context, path string) error {
	if !helper.IsChildAgent() || ctx.Request.Method != http.MethodPost && ctx.Request.Method != http.MethodPut && ctx.Request.Method != http.MethodPatch {
		return nil
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "apis" || parts[1] != "w7panel.w7.com" || parts[2] != "v1alpha1" || parts[3] != "permissions" {
		return nil
	}
	if ctx.Request.Method == http.MethodPost && len(parts) != 4 || ctx.Request.Method != http.MethodPost && len(parts) != 5 {
		return fmt.Errorf("不支持的权限资源写入路径")
	}
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, 2<<20+1))
	if err != nil || len(body) > 2<<20 {
		return fmt.Errorf("权限请求体无效或过大")
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	sdk := k8s.NewK8sClient().Sdk
	if ctx.Request.Method == http.MethodPatch {
		body, err = patchedPermissionObject(ctx.Request.Context(), sdk, parts[4], ctx.GetHeader("Content-Type"), body)
		if err != nil {
			return err
		}
	}
	var object struct {
		Spec struct {
			DomainWhiteList []configv1alpha1.DomainWhiteItem `json:"domainWhiteList"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &object); err != nil {
		return err
	}
	if ctx.Request.Method == http.MethodPost {
		if len(object.Spec.DomainWhiteList) != 0 {
			return fmt.Errorf("子集群域名白名单由主集群统一管理")
		}
		return nil
	}
	current, err := permissionservice.Get(ctx.Request.Context(), sdk, parts[4])
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current.Spec.DomainWhiteList, object.Spec.DomainWhiteList) && !(len(current.Spec.DomainWhiteList) == 0 && len(object.Spec.DomainWhiteList) == 0) {
		return fmt.Errorf("子集群域名白名单由主集群统一管理")
	}
	return nil
}

func patchedPermissionObject(ctx context.Context, sdk *k8s.Sdk, name, contentType string, patch []byte) ([]byte, error) {
	current, err := sdk.DynamicClient().Resource(schema.GroupVersionResource{Group: "w7panel.w7.com", Version: "v1alpha1", Resource: "permissions"}).Get(ctx, name, metav1.GetOptions{})
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
