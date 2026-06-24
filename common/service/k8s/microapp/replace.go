package microapp

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s/k3k"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/w7panel/w7panel/common/service/oidc"
	microapp "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
)

type MicroAppReplace struct {
	*k3ktypes.K3kUser
}

func NewMicroAppReplace(token string) (*MicroAppReplace, error) {
	k3kuser, err := k3k.TokenToK3kUser(token)
	if err != nil {
		return nil, err
	}
	return &MicroAppReplace{
		K3kUser: k3kuser,
	}, nil
}

/*
*
appid

userid

openid

nickname

role

is_installer

access_token
*/
func (m *MicroAppReplace) Replace(ctx context.Context, data map[string]string, role string, microapp *microapp.MicroApp) map[string]string {
	result := map[string]string{}
	requireAccessToken := false
	requireCloudAccessToken := false
	for _, v := range data {
		if strings.Contains(v, "${system.access_token}") {
			requireAccessToken = true
		}
		if strings.Contains(v, "${system.cloud_accesstoken}") {
			requireCloudAccessToken = true
		}
	}
	accessToken := ""
	if requireAccessToken {
		token, err := m.getAccessToken(ctx)
		if err != nil {
			return result
		}
		accessToken = token
	}
	cloudAccToken := ""
	if requireCloudAccessToken {
		token, err := GetCloudAccessToken(m.GetConsoleOpenId())
		if err != nil {
			slog.Error("GetCloudAccessToken", "err", err)
		}
		cloudAccToken = token
	}
	for k, v := range data {
		newVal := v
		newVal = strings.ReplaceAll(newVal, "${system.group}", microapp.Name)
		newVal = strings.ReplaceAll(newVal, "${system.userid}", m.Name)
		newVal = strings.ReplaceAll(newVal, "${system.openid}", m.GetConsoleOpenId())
		newVal = strings.ReplaceAll(newVal, "${system.nickname}", m.GetNickName())
		newVal = strings.ReplaceAll(newVal, "${system.role}", role)
		newVal = strings.ReplaceAll(newVal, "${system.url}", microapp.RoleServerUrl(role))
		// newVal = strings.ReplaceAll(newVal, "${system.installer}", m.Name)
		newVal = strings.ReplaceAll(newVal, "${system.access_token}", accessToken)
		newVal = strings.ReplaceAll(newVal, "${system.cloud_uid}", m.GetConsoleId())
		newVal = strings.ReplaceAll(newVal, "${system.cloud_accesstoken}", cloudAccToken)
		result[k] = newVal
	}
	return result
}

func (m *MicroAppReplace) getAccessToken(ctx context.Context) (string, error) {
	server, err := oidc.GetServer()
	if err != nil {
		return "", err
	}
	return server.CreateDefaultAccessToken(ctx, m.Name)
}

func GetCloudAccessToken(openId string) (string, error) {
	if openId != "" {
		// 获取 passport token
		result, err := helper.Remember(openId, time.Hour, func() (any, error) {
			token, err := console.OpenIdToPassportToken(openId)
			if err != nil {
				return "", err
			}
			return token.Token, err
		})
		if err != nil {
			return "", err
		}
		return result.(string), nil
	}
	return "", errors.New("openId is empty")
}
