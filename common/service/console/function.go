package console

import (
	"errors"
	"log/slog"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func RefreshCDTokenUseOpenid(saName string) error {
	sdk := k8s.NewK8sClient()
	respo := config.NewW7ConfigRepository(sdk.Sdk)
	if saName == "" {
		return nil
	}
	config, err := respo.Get(saName)
	if err != nil {
		slog.Error("获取配置失败RefreshCDTokenUseOpenid", "error", err)
		return nil
	}
	if config.UserInfo != nil && config.UserInfo.OpenId != "" {
		token, err := OpenIdToCdToken(config.UserInfo.OpenId)
		if err != nil {
			return err
		}
		config.ThirdpartyCDToken = token.Token
		return respo.Set(config)
	}
	return nil

}

func OpenIdToCloudAccessToken(openId string) (*PassportToken, error) {
	sdkclient, err := NewDefaultSdkClient()
	if err != nil {
		return nil, err
	}
	return sdkclient.OpenIdToCloudAccessToken(openId)
}

// GetCachedCloudAccessToken returns the cloud access token for an OpenID. The
// token is shared by several panel paths, so cache it to avoid repeatedly
// calling the cloud API during a single token lifetime.
func GetCachedCloudAccessToken(openId string) (string, error) {
	if openId == "" {
		return "", errors.New("openId is empty")
	}
	result, err := helper.RememberWithTTL("cloud-access-token:"+openId, func() (interface{}, time.Duration, error) {
		token, err := OpenIdToCloudAccessToken(openId)
		if err != nil {
			return "", 0, err
		}
		if token == nil || token.Token == "" {
			return "", 0, errors.New("cloud access token is empty")
		}
		return token.Token, cloudAccessTokenCacheTTL(token.ExpireTime), nil
	})
	if err != nil {
		return "", err
	}
	accessToken, ok := result.(string)
	if !ok {
		return "", errors.New("invalid cached cloud access token")
	}
	return accessToken, nil
}

const cloudAccessTokenRefreshWindow = time.Minute

// cloudAccessTokenCacheTTL expires cached values before the cloud token does.
// The cloud API returns a Unix timestamp in seconds. Tokens that expire within
// the safety window remain usable for this request but are not cached.
func cloudAccessTokenCacheTTL(expireTime int64) time.Duration {
	if expireTime <= 0 {
		return time.Hour
	}
	ttl := time.Until(time.Unix(expireTime, 0)) - cloudAccessTokenRefreshWindow
	if ttl <= 0 {
		return 0
	}
	return ttl
}

func OpenIdToCloudCode(openId, componentAppid string) (*PassportCode, error) {
	sdkclient, err := NewDefaultSdkClient()
	if err != nil {
		return nil, err
	}
	return sdkclient.OpenIdToCloudCode(openId, componentAppid)
}
