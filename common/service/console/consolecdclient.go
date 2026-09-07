package console

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"

	"github.com/go-resty/resty/v2"
	"github.com/w7panel/w7panel/common/helper"
)

var consoleApi = "https://console.w7.cc"

type AppSecret struct {
	AppId     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type PreInstall struct {
	ZipURL      string `json:"zip_url"`
	ReleaseName string `json:"release_name"`
}

type CertVerify struct {
	Verify bool   `json:"verify"`
	Sign   string `json:"sign"`
}

type PayResult struct {
	NeedPay      bool   `json:"needPay"`
	Ticket       string `json:"ticket"`
	OrderSn      string `json:"ipOrderSn"`
	CvmName      string `json:"cvmName"`
	CvmNamespace string `json:"cvmNamespace"`
}

// declared 使用K3kOrder
type OrderInfo struct {
	OrderStatus string `json:"ip_order_status"` // 控制台接口直接数据库返回的字段 paid return
	OrderSn     string `json:"ip_order_sn"`     // 订单号
	BuyMode     string `json:"buymode"`         // base expand
	Hour        string `json:"hour"`            // 购买时长
	Cpu         int64  `json:"cpu"`             // cpu
	Memory      int64  `json:"memory"`          //
	Storage     int64  `json:"storage"`         //
	Bandwidth   int64  `json:"bandwidth"`       // 带宽
	CvmName     string `json:"cvm_name"`        // cvm名称
}

func (c *OrderInfo) GetHour() int64 {
	return helper.FloatStringToInt64(c.Hour)
}

type GoodsProduct struct {
	GoodsId   int32 `json:"goodsId"`
	ProductId int32 `json:"productId"`
}

var (
	// ConsoleApi                     = "http://172.16.1.126:9004"
	ConsoleCDBaseApi                   = consoleApi + "/api/thirdparty-cd"
	ConsoleCDK8sOfflineApi             = consoleApi + "/api/thirdparty-cd/k8s-offline"
	ConsoleCDTokenConvert              = consoleApi + "/api/thirdparty-cd/token-convert"                    // 转换token接口
	ConsoleCDTokenRefresh              = consoleApi + "/api/thirdparty-cd/token/refresh"                    // 刷新token接口
	ConsoleCDPanelOrderApi             = consoleApi + "/api/thirdparty-cd/k8s-offline/order"                // 站点授权订单接口
	ConsoleCDPanelPrepareProductApi    = consoleApi + "/api/thirdparty-cd/k8s-offline/prepare"              // 站点授权准备产品接口
	ConsoleCDPanelLicenseSiteApi       = consoleApi + "/api/thirdparty-cd/k8s-offline/license/register"     // 站点授权创建站点接口
	ConsoleCDPanelLicenseSiteZpkApi    = consoleApi + "/api/thirdparty-cd/k8s-offline/license/register-zpk" // 制品库创建站点接口
	ConsoleCDPanelResourceApi          = consoleApi + "/api/thirdparty-cd/k8s-offline/panel/resource"
	ConsoleCDPanelResourceApiSdk       = consoleApi + "/api/thirdparty-cd/sdk/k8s-offline/panel/resource"
	ConsoleCDPanelOpenidConvertApi     = consoleApi + "/api/thirdparty-cd/k8s-offline/openid-to-cd-token"
	ConsoleCDPanelOpenidConvertPassApi = consoleApi + "/api/thirdparty-cd/k8s-offline/openid-to-pass-access-token" //passport token
	ConsoleApiAccessTokenToCDToken     = consoleApi + "/register"
	ConfigSercret                      = "w7-config"
	AccessToken                        = "AccessToken" //oauth token
)

func SetConsoleApi(api string) {
	consoleApi = api
	ConsoleCDBaseApi = consoleApi + "/api/thirdparty-cd"
	ConsoleCDTokenConvert = consoleApi + "/api/thirdparty-cd/token-convert"
	ConsoleCDTokenRefresh = consoleApi + "/api/thirdparty-cd/token/refresh"
	ConsoleCDK8sOfflineApi = consoleApi + "/api/thirdparty-cd/k8s-offline"
	ConsoleCDPanelOrderApi = consoleApi + "/api/thirdparty-cd/k8s-offline/order"
	ConsoleCDPanelPrepareProductApi = consoleApi + "/api/thirdparty-cd/k8s-offline/prepare"
	ConsoleCDPanelLicenseSiteApi = consoleApi + "/api/thirdparty-cd/k8s-offline/license/register"        // 站点授权创建站点接口
	ConsoleCDPanelLicenseSiteZpkApi = consoleApi + "/api/thirdparty-cd/k8s-offline/license/register-zpk" // 制品库创建站点接口
	ConsoleCDPanelResourceApi = consoleApi + "/api/thirdparty-cd/k8s-offline/panel/resource"
	ConsoleCDPanelResourceApiSdk = consoleApi + "/api/thirdparty-cd/sdk/k8s-offline/panel/resource"
	ConsoleCDPanelOpenidConvertApi = consoleApi + "/api/thirdparty-cd/k8s-offline/openid-to-cd-token"              // 转换openid到cd token接口
	ConsoleCDPanelOpenidConvertPassApi = consoleApi + "/api/thirdparty-cd/k8s-offline/openid-to-pass-access-token" //passport token
	ConsoleApiAccessTokenToCDToken = consoleApi + "/register"
	ConfigSercret = "w7-config"
	AccessToken = "AccessToken" //oauth token
}

type ConsoleCdClient struct {
	token  string
	client *resty.Client
}

type CDToken struct {
	Token string `json:"token"`
}

func NewConsoleCdClient(token string) *ConsoleCdClient {
	req := helper.RetryHttpClient()
	// req.Debug = true
	// req.Debug = config.GetBool(ConfigSercret, "debug")
	req.SetHeader("Accept", "application/json")
	return &ConsoleCdClient{client: req, token: token}
}

func (c *ConsoleCdClient) SetToken(token string) {
	c.token = token
	// c.client.AddHeader("Authorization", "Bearer "+token)
}
func (c *ConsoleCdClient) CreateSite(domainUrl string, releaseName string) (*AppSecret, error) {
	secret := &AppSecret{}
	consoleErr := &ConsoleError{}
	urlvalues := url.Values{}
	urlvalues.Add("appName", releaseName)
	urlvalues.Add("domain_host", domainUrl)
	updateUrl := ConsoleCDK8sOfflineApi + "/create-site"
	response, err := c.client.R().SetAuthToken(c.token).SetFormDataFromValues(urlvalues).SetResult(secret).SetError(consoleErr).Post(updateUrl)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		return nil, consoleErr
	}
	return secret, err
}

func (c *ConsoleCdClient) PreInstall(consoleurl string) (*PreInstall, error) {
	result := &PreInstall{}
	errorResult := &ConsoleError{}
	urlvalues := url.Values{}
	urlvalues.Add("url", consoleurl)
	// urlvalues.Add("cluster_id", clusterId)
	updateUrl := ConsoleCDK8sOfflineApi + "/pre-install"
	// slog.Info("pre install", "url", updateUrl, "clusterId", clusterId)
	response, err := c.client.R().SetAuthToken(c.token).SetFormDataFromValues(urlvalues).SetResult(result).SetError(errorResult).Post(updateUrl)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		slog.Warn("pre install error", "statusCode", response.StatusCode(), "response", response.String())
		return nil, response.Error().(error)
	}
	return result, err
}

func (c *ConsoleCdClient) CreatePanelOrder(urlValues url.Values) (*PayResult, error) {
	result := &PayResult{}
	cerr := &ConsoleError{}
	response, err := c.client.R().SetAuthToken(c.token).SetFormDataFromValues(urlValues).SetResult(result).SetError(cerr).Post(ConsoleCDPanelOrderApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		slog.Warn("CreatePanelOrder error", "statusCode", response.StatusCode(), "response", response.String())
		return nil, cerr
	}
	return result, err
}

func (c *ConsoleCdClient) GetPanelOrderInfo(urlValues url.Values) (*OrderInfo, error) {
	result := &OrderInfo{}
	response, err := c.client.R().SetAuthToken(c.token).SetQueryParamsFromValues(urlValues).SetResult(result).Get(ConsoleCDPanelOrderApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		return nil, errors.New("GetPanelOrderInfo error" + response.String())
	}
	return result, err
}

func (c *ConsoleCdClient) PrepareProduct() (*GoodsProduct, error) {
	result := &GoodsProduct{}
	response, err := c.client.R().SetAuthToken(c.token).SetResult(result).Post(ConsoleCDPanelPrepareProductApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		slog.Warn("prepare product error", "statusCode", response.StatusCode(), "response", response.String())
		return nil, errors.New("prepare product error" + response.String())
	}
	return result, err
}

func (c *ConsoleCdClient) PublishPanelResource(urlValues map[string]string) error {
	response, err := c.client.R().SetAuthToken(c.token).SetFormData(urlValues).Post(ConsoleCDPanelResourceApi)
	if err != nil {
		return err
	}
	if response.StatusCode() > 299 {
		return errors.New("PublishPanelResource error" + response.String())
	}
	return nil
}

func (c *ConsoleCdClient) DeletePanelResource(urlValues map[string]string) error {
	response, err := c.client.R().SetAuthToken(c.token).SetFormData(urlValues).Delete(ConsoleCDPanelResourceApi)
	if err != nil {
		return err
	}
	if response.StatusCode() > 299 {
		return errors.New("DeletePanelResource error" + response.String())
	}
	return nil
}

func (c *ConsoleCdClient) CreateLicenseSite(urlValues map[string]string) (*License, error) {
	result := &License{}
	err2 := &ConsoleError{}
	response, err := c.client.R().SetAuthToken(c.token).SetFormData(urlValues).SetResult(result).SetError(err2).Post(ConsoleCDPanelLicenseSiteApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 399 {
		slog.Warn("CreateLicenseSite error", "statusCode", response.StatusCode(), "response", response.String())
		return nil, response.Error().(error)
	}
	return result, nil
}

// 制品库创建站点接口
func (c *ConsoleCdClient) CreateLicenseSiteZpk(urlValues map[string]string) (*License, error) {
	result := &License{}
	err2 := &ConsoleError{}
	response, err := c.client.R().SetAuthToken(c.token).SetFormData(urlValues).SetResult(result).SetError(err2).Post(ConsoleCDPanelLicenseSiteZpkApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 399 {
		slog.Warn("CreateLicenseSite error", "statusCode", response.StatusCode(), "response", response.String())
		return nil, response.Error().(error)
	}
	return result, nil
}

// TokenRefreshResponse is returned by the console token refresh API.
type TokenRefreshResponse struct {
	Refresh bool   `json:"refresh"`
	Message string `json:"message"`
	Token   string `json:"token"`
}

func (c *ConsoleCdClient) RefreshToken() (*TokenRefreshResponse, error) {
	var result TokenRefreshResponse
	updateUrl := ConsoleCDTokenRefresh
	urlvalues := url.Values{}
	urlvalues.Add("panel_version", os.Getenv("HELM_VERSION"))
	response, err := c.client.R().SetAuthToken(c.token).SetFormDataFromValues(urlvalues).SetResult(&result).Put(updateUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	if response.StatusCode() > 299 {
		return nil, fmt.Errorf("refresh token failed with status %d: %s",
			response.StatusCode(), response.String())
	}

	if !result.Refresh {
		if result.Message != "" {
			return nil, fmt.Errorf("refresh token failed: %s", result.Message)
		}
		return nil, errors.New("refresh token failed: unknown error")
	}

	return &result, nil
}

func AccessTokenToCDToken(token string) (*CDToken, error) {

	urlvalues := url.Values{}
	urlvalues.Add("accesstoken", token)
	result := &CDToken{}
	response, err := helper.RetryHttpClient().R().SetHeader("Accept", "application/json").SetQueryParamsFromValues(urlvalues).SetResult(result).Get(ConsoleCDTokenConvert)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() != 200 {
		slog.Error("获取token失败", "err", err)
		return nil, errors.New("获取token失败" + response.String())
	}
	return result, nil
}

func CreateLicenseSite(accessToken string, offlineUrl string) (*License, error) {
	result := &License{}
	urlvalues := url.Values{}
	urlvalues.Add("accesstoken", accessToken)
	urlvalues.Add("offlineUrl", offlineUrl)
	response, err := helper.RetryHttpClient().R().SetFormDataFromValues(urlvalues).SetResult(result).Post(ConsoleCDPanelLicenseSiteApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		return nil, errors.New("CreateLicense error" + response.String())
	}
	return result, nil
}

func OpenIdToCdToken(openId string) (*ThirdPartyCDToken, error) {
	result := &ThirdPartyCDToken{}
	urlvalues := url.Values{}
	urlvalues.Add("openid", openId)
	urlvalues.Add("useDefaultAppid", "1")
	response, err := helper.RetryHttpClient().R().SetFormDataFromValues(urlvalues).SetResult(result).Post(ConsoleCDPanelOpenidConvertApi)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		return nil, errors.New("OpenIdToCdToken error" + response.String())
	}
	return result, nil
}

func VerifyCert(cert *x509.Certificate) (*CertVerify, error) {

	if len(cert.Subject.OrganizationalUnit) == 0 {
		return nil, errors.New("证书组织单位缺失")
	}
	req := helper.RetryHttpClient()
	// req.Debug = config.GetBool(ConfigSercret, "debug")
	req.SetHeader("Accept", "application/json")
	// 获取证书摘要 并校验
	verifyRes := &CertVerify{}
	urlvalues := url.Values{}

	random := helper.RandomString(10)
	pubkey := cert.PublicKey.(*rsa.PublicKey)

	encryptedBase64, err := helper.EncryptWithPublicKey(pubkey, []byte(random), true)
	if err != nil {
		return nil, err
	}
	// encryptedBase64 := base64.StdEncoding.EncodeToString(encryptedData)
	urlvalues.Add("random", random)
	urlvalues.Add("random_enc", string(encryptedBase64))
	certId := cert.SerialNumber.String()
	certUrl := ConsoleCDK8sOfflineApi + "/license/" + certId + "/verify-cert"
	response, err := req.R().SetFormDataFromValues(urlvalues).SetResult(verifyRes).Post(certUrl)
	if err != nil {
		return nil, err
	}
	if response.StatusCode() > 299 {
		return nil, errors.New("校验证书失败" + response.String())
	}
	if !verifyRes.Verify {
		return nil, errors.New("校验证书失败")
	}
	decodeBase64, err := base64.StdEncoding.DecodeString(verifyRes.Sign)
	if err != nil {
		return nil, err
	}
	// if (cert.Subject.OrganizationalUnit != "w7") && (cert.Subject.OrganizationalUnit != "w7-enterprise")
	err = helper.VerifyDataWithPublicKey(pubkey, []byte(cert.Subject.OrganizationalUnit[0]), decodeBase64)
	if err != nil {
		return nil, err
	}
	return verifyRes, err
}
