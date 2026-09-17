package console

import (
	"errors"
	"log/slog"
	"time"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
)

func VerifyLicense(license *License, clean bool) error {
	if license == nil {
		return errors.New("证书为空")
	}
	if license.License == nil {
		return errors.New("证书为空")
	}
	cert := license.License
	// if cert.Issuer.Organization != nil && len(cert.Issuer.Organization) > 0 {
	// 	if cert.Issuer.Organization[0] != license.AppId {
	// 		cleanLicenseCert(clean)
	// 		return errors.New("证书颁发者与授权不符")
	// 	}
	// }
	_, err := VerifyCert(cert)
	if err != nil {
		cleanLicenseCert(clean)
		return err
	}
	SetCurrentLicense(license)
	return nil
}

func cleanLicenseCert(clean bool) {
	SetCurrentLicense(nil)
	if clean {
		client, err := NewDefaultLicenseClient()
		if err != nil {
			slog.Error("清理证书失败", "error", err)
			return
		}
		err = client.CleanLicense()
		if err != nil {
			slog.Error("清理证书失败x", "error", err)
			return
		}
	}
}

func VerifyDefaultLicense(clean bool) error {
	license := GetCurrentLicense()
	err := VerifyLicense(license, clean)
	if err != nil {
		slog.Error("验证证书失败", "error", err)
		return err
	}
	return nil
}

func VerifyLicenseId(id, saName string) error {
	licenseClient, err := NewDefaultLicenseClient()
	if err != nil {
		return err
	}
	license, err := licenseClient.GetLicense()
	if err != nil {
		return err
	}
	sdkClient, err := NewSdkClient(license)
	if err != nil {
		return err
	}
	licenseOrder, err := sdkClient.GenCert(id)
	if err != nil {
		return err
	}

	err = licenseClient.ImportCert([]byte(licenseOrder.Cert), saName)
	if err != nil {
		return err
	}
	return nil
}

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

func RefreshCDToken() error {
	repository := config.NewW7ConfigRepository(k8s.NewK8sClientInner())
	configs, err := repository.List()
	if err != nil {
		return err
	}
	for _, v := range configs {
		err = RefreshW7Config(v, repository)
		if err != nil {
			slog.Error("刷新CDToken失败", "error", err)
		}
		// err = ReVerifyLicense(v, repository)
		// if err != nil {
		// 	slog.Error("验证证书失败", "error", err)
		// }

		if v.ClusterId != "0" && v.ClusterId != "" && v.ThirdpartyCDToken != "" && v.NotFree() {
			config.MainW7Config = v
			// 应该判断v.Name != "main"
			// clone := v.Clone()
			// clone.Name = "main"
			// repository.Set(clone)
		}

	}

	return nil
}
func RefreshW7Config(w7config *config.W7Config, repository config.W7ConfigRepositoryInterface) error {
	token := w7config.ThirdpartyCDToken
	if token == "" || token == "0" {
		return errors.New("未配置CDToken")
	}
	cdclient := NewConsoleCdClient(token)
	if !w7config.IsCDTokenWillExpired() {
		slog.Info("无需刷新CDToken")
		// return nil
	}
	tokenResult, err := cdclient.RefreshToken()
	if err != nil {
		slog.Error("刷新CDToken失败", "error", err)
		return err
	}
	if tokenResult.Refresh && tokenResult.Token != "" {
		w7config.ThirdpartyCDToken = tokenResult.Token
		w7config.CDTokenExpireTime = int(time.Now().Add(time.Minute * 55).Unix())
		err := repository.Set(w7config)
		if err != nil {
			slog.Error("更新auth配置失败", "error", err)
			return err
		}
		slog.Info("刷新token成功")
		slog.Info("刷新CDToken成功", "name", w7config.Name, "token", tokenResult.Token)
		getConfig, err := repository.Get(w7config.Name)
		if err != nil {
			slog.Error("获取配置失败", "error", err)
		}
		if getConfig.ThirdpartyCDToken != tokenResult.Token {
			slog.Error("token not eq", "apitoken", tokenResult.Token, "configtoken", getConfig.ThirdpartyCDToken)
			return nil
		}

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
