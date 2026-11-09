package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func loadMainSync(ctx context.Context, path string, dst interface{}) error {
	endpoint := strings.TrimRight(os.Getenv("CKM_SYNC_ENDPOINT"), "/")
	token := os.Getenv("CKM_SYNC_TOKEN")
	if endpoint == "" || token == "" {
		return fmt.Errorf("主集群域名白名单同步通道未配置")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-W7Panel-CKM-Sync-Token", token)
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("主集群域名白名单同步失败: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(dst)
}

func LoadMainDomainWhiteList(ctx context.Context, permissionName string) ([]configv1alpha1.DomainWhiteItem, error) {
	if !helper.IsChildAgent() {
		if permissionName == "" {
			return nil, fmt.Errorf("当前用户权限未确定")
		}
		permission, err := Get(ctx, k8s.NewK8sClient().Sdk, permissionName)
		if apierrors.IsNotFound(err) {
			return []configv1alpha1.DomainWhiteItem{}, nil
		}
		if err != nil {
			return nil, err
		}
		if permission.Spec.DomainWhiteList == nil {
			return []configv1alpha1.DomainWhiteItem{}, nil
		}
		return permission.Spec.DomainWhiteList, nil
	}
	var response struct {
		DomainWhiteList []configv1alpha1.DomainWhiteItem `json:"domainWhiteList"`
	}
	if err := loadMainSync(ctx, "/domain-white-list", &response); err != nil {
		return nil, err
	}
	if response.DomainWhiteList == nil {
		return nil, fmt.Errorf("主集群域名白名单响应不完整")
	}
	return response.DomainWhiteList, nil
}
