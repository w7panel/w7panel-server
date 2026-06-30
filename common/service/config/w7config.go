package config

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	service "github.com/w7corp/sdk-open-cloud-go/service"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func init() {
	CurrentCity, _ = helper.MyCity() // 当前服务器所在的城市
	// CurrentCity = "北京"
}

type licenseVerify struct {
	sync.Map
}

func (l *licenseVerify) IsCompany(name string) bool {
	value, ok := l.Load(name)
	if ok {
		return value.(string) == "company"
	}
	return false
}

func (l *licenseVerify) IsTeam(name string) bool {
	value, ok := l.Load(name)
	if ok {
		return value.(string) == "team"
	}
	return false
}

var LicenseVerify = licenseVerify{}

// company team类型 取ServiceAccount 其中一个
var MainW7Config *W7Config

var CurrentCity string

var userGVR = schema.GroupVersionResource{
	Group:    "w7panel.w7.com",
	Version:  "v1alpha1",
	Resource: "users",
}

func SetVerifyType(name, verifyType string) {
	LicenseVerify.Store(name, verifyType)
}

type W7Config struct {
	Name              string                  `json:"name"`
	ThirdpartyCDToken string                  `json:"thirdparty_cd_token"`
	CDTokenExpireTime int                     `json:"cd_token_expire_time"`
	ClusterId         string                  `json:"cluster_id"`
	OfflineUrl        string                  `json:"offline_url"`
	AccessToken       string                  `json:"access_token"`
	ExpireTime        int                     `json:"expire_time"`
	ApiServerUrl      string                  `json:"api_server_url"`
	UserInfo          *service.ResultUserinfo `json:"user_info"`
	License           *x509.Certificate       `json:"license"`
	DebugValue        string                  `json:"debug_value"`
}

func NewEmptyConfig() *W7Config {
	return &W7Config{
		ThirdpartyCDToken: "",
		ClusterId:         "",
		AccessToken:       "",
		ExpireTime:        0,
		ApiServerUrl:      "",
	}
}

func (c *W7Config) Clone() *W7Config {
	return &W7Config{
		Name:              c.Name,
		ThirdpartyCDToken: c.ThirdpartyCDToken,
		CDTokenExpireTime: c.CDTokenExpireTime,
		ClusterId:         c.ClusterId,
		OfflineUrl:        c.OfflineUrl,
		AccessToken:       c.AccessToken,
		ExpireTime:        c.ExpireTime,
		ApiServerUrl:      c.ApiServerUrl,
		UserInfo:          c.UserInfo,
		License:           c.License,
		DebugValue:        c.DebugValue,
	}
}

func (c *W7Config) IsWillExpired() bool {
	return int64(c.ExpireTime) < time.Now().Unix()+300
}

func (c *W7Config) IsExpired() bool {
	return int64(c.ExpireTime) < time.Now().Unix()
}
func (c *W7Config) IsCDTokenWillExpired() bool {
	return int64(c.CDTokenExpireTime)-600 < time.Now().Unix()
}

func (c *W7Config) GetLicenseType() string {
	license := c.License
	licenseType := "free"
	isExpired := false
	if license != nil && len(license.Subject.Province) > 0 {
		licenseType = license.Subject.Province[0]
	}
	if license != nil {
		// endTime = license.NotAfter
		if license.NotAfter.Before(time.Now()) {
			isExpired = true
		}
		if isExpired {
			licenseType = "free"
		}
	}
	return licenseType
}

func (c *W7Config) NotFree() bool {
	licenseType := c.GetLicenseType()
	return licenseType == "team" || licenseType == "company"
}

func (c *W7Config) ToArray() map[string]interface{} {
	license := c.License
	licenseType := "free"
	licenseId := "0"
	if license != nil && len(license.Subject.Province) > 0 {
		licenseType = license.Subject.Province[0]
	}
	endTime := time.Now()
	isExpired := false
	if license != nil {
		licenseId = license.SerialNumber.String()
		endTime = license.NotAfter
		if license.NotAfter.Before(time.Now()) {
			isExpired = true
		}
		if isExpired {
			licenseType = "free"
		}
	}

	return map[string]interface{}{
		"thirdparty_cd_token": c.ThirdpartyCDToken,
		"cluster_id":          c.ClusterId,
		"offline_url":         c.OfflineUrl,
		"access_token":        c.AccessToken,
		"expire_time":         c.ExpireTime,
		"api_server_url":      c.ApiServerUrl,
		"require_oauth":       c.UserInfo == nil,
		"is_register":         c.ClusterId != "",
		"userinfo":            c.UserInfo,
		"license_type":        licenseType,
		"license_id":          licenseId,
		"license_end_time":    endTime.Format("2006-01-02 15:04:05"),
		"license_is_expired":  isExpired,
		"debug_value":         c.DebugValue,
		// "license":             c.License.Raw,
	}
}

type W7ConfigRepositoryInterface interface {
	Get(name string) (*W7Config, error)
	Set(w7config *W7Config) error
	GetByConsoleId(consoleId string) (*W7Config, error)

	List() ([]*W7Config, error)
}

type w7ConfigRepository struct {
	*k8s.Sdk
}

func NewW7ConfigRepository(sdk *k8s.Sdk) *w7ConfigRepository {
	return &w7ConfigRepository{sdk}
}

func secretName(name string) string {
	return name + ".w7-config"
}

func (c *w7ConfigRepository) secretToW7config(secret *v1.Secret, name string) *W7Config {
	expireTimeStr := string(secret.Data["expire_time"])
	expireTime, err := strconv.Atoi(expireTimeStr)
	if err != nil {
		expireTime = 0
		//return W7Config{}, fmt.Errorf("failed to convert expire_time to int: %w", err)
	}
	cdExpireTime, err := strconv.Atoi(string(secret.Data["cd_token_expire_time"]))
	if err != nil {
		cdExpireTime = 0
	}
	userInfoStr, ok := secret.Data["userinfo"]
	userInfo := service.ResultUserinfo{}
	if ok {
		err = json.Unmarshal(userInfoStr, &userInfo)
		if err != nil {
			slog.Warn("failed to unmarshal userinfo: %w", "err", err)
		}
	}
	var x509Certificate *x509.Certificate = nil
	if len(secret.Data["license"]) > 0 {
		cert, err := helper.ParseX509(secret.Data["license"])
		if err == nil {
			x509Certificate = cert
		}
	}

	return &W7Config{
		Name:              name,
		ThirdpartyCDToken: string(secret.Data["thirdparty_cd_token"]),
		CDTokenExpireTime: cdExpireTime,
		AccessToken:       string(secret.Data["access_token"]),
		ExpireTime:        expireTime,
		ClusterId:         string(secret.Data["cluster_id"]),
		OfflineUrl:        string(secret.Data["offline_url"]),
		UserInfo:          &userInfo,
		License:           x509Certificate,
		DebugValue:        string(secret.Labels["w7.cc/test"]),
	}
}

func (c *w7ConfigRepository) Get(name string) (*W7Config, error) {
	obj, err := c.DynamicClient().Resource(userGVR).Get(c.Ctx, name, metav1.GetOptions{})
	if err != nil {
		return &W7Config{Name: name}, fmt.Errorf("failed to get user w7-config: %w", err)
	}
	config, ok, err := unstructured.NestedMap(obj.Object, "spec", "w7Config")
	if err != nil {
		return nil, err
	}
	if !ok {
		return &W7Config{Name: name}, fmt.Errorf("not found w7 config for user: %s", name)
	}
	return userConfigToW7Config(name, config)
}

func (c *w7ConfigRepository) List() ([]*W7Config, error) {
	list, err := c.DynamicClient().Resource(userGVR).List(c.Ctx, metav1.ListOptions{})
	if err != nil {
		return []*W7Config{}, fmt.Errorf("failed to list user w7-config: %w", err)
	}
	configs := []*W7Config{}
	for i := range list.Items {
		config, ok, err := unstructured.NestedMap(list.Items[i].Object, "spec", "w7Config")
		if err != nil || !ok {
			continue
		}
		cfg, err := userConfigToW7Config(list.Items[i].GetName(), config)
		if err == nil {
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

func (c *w7ConfigRepository) GetByConsoleId(consoleId string) (*W7Config, error) {
	list, err := c.DynamicClient().Resource(userGVR).List(c.Ctx, metav1.ListOptions{})
	if err != nil {
		return &W7Config{}, fmt.Errorf("failed to get user by console id: %w", err)
	}
	for i := range list.Items {
		currentConsoleId, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "consoleId")
		if currentConsoleId != consoleId {
			continue
		}
		config, ok, err := unstructured.NestedMap(list.Items[i].Object, "spec", "w7Config")
		if err != nil {
			return nil, err
		}
		if !ok {
			return &W7Config{Name: list.Items[i].GetName()}, fmt.Errorf("not found w7 config for user: %s", list.Items[i].GetName())
		}
		return userConfigToW7Config(list.Items[i].GetName(), config)
	}
	return &W7Config{}, apierrors.NewNotFound(schema.GroupResource{Group: userGVR.Group, Resource: userGVR.Resource}, consoleId)
}

func (c *w7ConfigRepository) Set(config *W7Config) error {
	if config.DebugValue == "" {
		config.DebugValue = helper.RandomString(5)
	}
	if err := c.setUserConfig(config); err != nil {
		return err
	}
	return c.setSecretConfig(config)
}

func (c *w7ConfigRepository) setSecretConfig(config *W7Config) error {
	secretName := secretName(config.Name)
	secret, err := c.ClientSet.CoreV1().Secrets(c.GetNamespace()).Get(c.Ctx, secretName, metav1.GetOptions{})
	isUpdate := true
	if err != nil {
		secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: c.GetNamespace(),
			},
			Data: map[string][]byte{},
			Type: v1.SecretTypeOpaque,
		}
		isUpdate = false
	}
	if secret.Labels == nil {
		secret.Labels = make(map[string]string)
	}
	secret.Labels["w7.cc/oauth-config"] = "true"
	secret.Labels["w7.cc/test"] = config.DebugValue
	secret.Data["thirdparty_cd_token"] = []byte(config.ThirdpartyCDToken)
	secret.Data["access_token"] = []byte(config.AccessToken)
	if config.ExpireTime == 0 {
		secret.Data["expire_time"] = []byte("0")
	}
	if config.CDTokenExpireTime > 0 {
		secret.Data["cd_token_expire_time"] = []byte(strconv.Itoa(config.CDTokenExpireTime))
	}
	secret.Data["expire_time"] = []byte(strconv.Itoa(config.ExpireTime))
	secret.Data["cluster_id"] = []byte(config.ClusterId)
	secret.Data["offline_url"] = []byte(config.OfflineUrl)
	if config.UserInfo != nil {
		userInfoBytes, err := json.Marshal(config.UserInfo)
		secret.Labels["w7.cc/console-uid"] = strconv.Itoa(config.UserInfo.UserId)
		if err == nil {
			secret.Data["userinfo"] = userInfoBytes
		}
	}
	if (config.License != nil) && len(config.License.Raw) > 0 {
		secret.Data["license"] = config.License.Raw
	}
	if !isUpdate {
		_, err = c.ClientSet.CoreV1().Secrets(c.GetNamespace()).Create(c.Ctx, secret, metav1.CreateOptions{})
	} else {
		_, err = c.ClientSet.CoreV1().Secrets(c.GetNamespace()).Update(c.Ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			slog.Warn("failed to update w7-config: %w", "err", err)
		}
	}
	return err
}

func (c *w7ConfigRepository) setUserConfig(config *W7Config) error {
	obj, err := c.DynamicClient().Resource(userGVR).Get(c.Ctx, config.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	configMap, err := w7ConfigToUserConfig(config)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(obj.Object, configMap, "spec", "w7Config"); err != nil {
		return err
	}
	if config.UserInfo != nil {
		consoleId, _, _ := unstructured.NestedString(obj.Object, "spec", "consoleId")
		if consoleId == "" {
			if err := unstructured.SetNestedField(obj.Object, strconv.Itoa(config.UserInfo.UserId), "spec", "consoleId"); err != nil {
				return err
			}
		}
		consoleOpenid, _, _ := unstructured.NestedString(obj.Object, "spec", "consoleOpenid")
		if consoleOpenid == "" {
			if err := unstructured.SetNestedField(obj.Object, config.UserInfo.OpenId, "spec", "consoleOpenid"); err != nil {
				return err
			}
		}
		consoleNickname, _, _ := unstructured.NestedString(obj.Object, "spec", "consoleNickname")
		if consoleNickname == "" {
			if err := unstructured.SetNestedField(obj.Object, config.UserInfo.Nickname, "spec", "consoleNickname"); err != nil {
				return err
			}
		}
	}
	_, err = c.DynamicClient().Resource(userGVR).Update(c.Ctx, obj, metav1.UpdateOptions{})
	return err
}

func w7ConfigToUserConfig(config *W7Config) (map[string]interface{}, error) {
	if config == nil {
		return nil, nil
	}
	license := ""
	if config.License != nil && len(config.License.Raw) > 0 {
		license = base64.StdEncoding.EncodeToString(config.License.Raw)
	}
	configMap := map[string]interface{}{
		"thirdpartyCDToken": config.ThirdpartyCDToken,
		"cdTokenExpireTime": int64(config.CDTokenExpireTime),
		"clusterId":         config.ClusterId,
		"offlineUrl":        config.OfflineUrl,
		"accessToken":       config.AccessToken,
		"expireTime":        int64(config.ExpireTime),
		"apiServerUrl":      config.ApiServerUrl,
		"license":           license,
		"debugValue":        config.DebugValue,
	}
	if config.UserInfo != nil {
		userInfo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config.UserInfo)
		if err != nil {
			return nil, err
		}
		configMap["userInfo"] = userInfo
	}
	return configMap, nil
}

func userConfigToW7Config(name string, config map[string]interface{}) (*W7Config, error) {
	if config == nil {
		return &W7Config{Name: name}, fmt.Errorf("not found w7 config for user: %s", name)
	}
	var cert *x509.Certificate
	license := stringValue(config["license"])
	if license != "" {
		raw, err := base64.StdEncoding.DecodeString(license)
		if err != nil {
			return nil, err
		}
		cert, err = helper.ParseX509(raw)
		if err != nil {
			return nil, err
		}
	}
	var userInfo *service.ResultUserinfo
	if rawUserInfo, ok := config["userInfo"].(map[string]interface{}); ok && len(rawUserInfo) > 0 {
		info := service.ResultUserinfo{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(rawUserInfo, &info); err != nil {
			return nil, err
		}
		userInfo = &info
	}
	return &W7Config{
		Name:              name,
		ThirdpartyCDToken: stringValue(config["thirdpartyCDToken"]),
		CDTokenExpireTime: intValue(config["cdTokenExpireTime"]),
		ClusterId:         stringValue(config["clusterId"]),
		OfflineUrl:        stringValue(config["offlineUrl"]),
		AccessToken:       stringValue(config["accessToken"]),
		ExpireTime:        intValue(config["expireTime"]),
		ApiServerUrl:      stringValue(config["apiServerUrl"]),
		UserInfo:          userInfo,
		License:           cert,
		DebugValue:        stringValue(config["debugValue"]),
	}, nil
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intValue(v interface{}) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case int32:
		return int(value)
	case float64:
		return int(value)
	case string:
		i, _ := strconv.Atoi(value)
		return i
	default:
		return 0
	}
}

func (c *w7ConfigRepository) MigrateSecretsToUsers(ctx context.Context) error {
	secrets, err := c.ClientSet.CoreV1().Secrets(c.GetNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: "w7.cc/oauth-config=true",
	})
	if err != nil {
		return fmt.Errorf("failed to list w7-config secrets: %w", err)
	}
	for i := range secrets.Items {
		secret := &secrets.Items[i]
		if !strings.HasSuffix(secret.Name, ".w7-config") {
			continue
		}
		cfg := c.secretToW7config(secret, strings.TrimSuffix(secret.Name, ".w7-config"))
		if err := c.setUserConfig(cfg); err != nil {
			if apierrors.IsNotFound(err) {
				slog.Warn("skip w7-config secret because user not found", "secret", secret.Name, "user", cfg.Name)
				continue
			}
			return err
		}
	}
	return nil
}
