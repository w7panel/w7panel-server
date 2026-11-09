package site

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	microappsetting "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"
)

var ErrMainSettingsUnavailable = errors.New("主集群配置读取通道未配置")

// FetchMainSettings reads the public display fields exposed by the main cluster sync endpoint.
func FetchMainSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	endpoint := strings.TrimRight(os.Getenv("CKM_SYNC_ENDPOINT"), "/")
	token := os.Getenv("CKM_SYNC_TOKEN")
	if endpoint == "" || token == "" {
		return nil, ErrMainSettingsUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/main-settings", nil)
	if err != nil {
		return nil, fmt.Errorf("主集群配置读取地址无效: %w", err)
	}
	req.Header.Set("X-W7Panel-CKM-Sync-Token", token)
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("读取主集群配置失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("读取主集群配置失败: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Code int                        `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil || payload.Code != http.StatusOK {
		return nil, errors.New("主集群配置响应无效")
	}
	for _, key := range []string{"siteSetting", "configMaps", "domainParse"} {
		if _, ok := payload.Data[key]; !ok {
			return nil, errors.New("主集群配置响应无效")
		}
	}
	return payload.Data, nil
}

func MainSiteSetting(data map[string]json.RawMessage) (*microappsetting.MicroAppSetting, error) {
	if len(data["siteSetting"]) == 0 || string(data["siteSetting"]) == "null" {
		return nil, nil
	}
	var setting microappsetting.MicroAppSetting
	if err := json.Unmarshal(data["siteSetting"], &setting); err != nil {
		return nil, err
	}
	return &setting, nil
}
