package k8s

import (
	"sync"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"
	"github.com/w7panel/w7panel/common/helper"
)

var tokenCache = make(map[string]int64)
var lock sync.Mutex

type K8sToken struct {
	token        string
	mu           sync.RWMutex
	claims       jwtv5.MapClaims
	claimsErr    error
	claimsParsed bool
}

type K3kConfig struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	ApiServer string `json:"apiServer"`
	CvmName   string `json:"cvmName"`
}

func NewK3kConfig(name, namespace, apiServer, cvmName string) *K3kConfig {
	return &K3kConfig{
		Name:      name,
		Namespace: namespace,
		ApiServer: apiServer,
		CvmName:   cvmName,
	}
}

func (u *K3kConfig) GetVirtualIngressServiceName() string {
	return helper.GetVirtualIngressServiceName(u.Namespace, u.CvmName)
}

func NewK8sToken(token string) *K8sToken {
	return &K8sToken{
		token: token,
	}
}

// parseClaims 解析并缓存 JWT claims
func (t *K8sToken) parseClaims() (jwtv5.MapClaims, error) {
	t.mu.RLock()
	if t.claimsParsed {
		t.mu.RUnlock()
		return t.claims, t.claimsErr
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()

	// 双重检查
	if t.claimsParsed {
		return t.claims, t.claimsErr
	}

	data := jwtv5.MapClaims{}
	jwtToken, _, err := jwtv5.NewParser().ParseUnverified(t.token, data)
	if err != nil {
		t.claimsErr = err
		t.claimsParsed = true
		return nil, err
	}
	t.claims = data
	t.claimsParsed = true
	_ = jwtToken // 仅用于验证，无需使用
	return t.claims, nil
}

func (t *K8sToken) Cache() error {
	expiretime, err := t.GetExpireTime()
	if err != nil {
		return err
	}
	if expiretime == nil {
		return errors.New("expiretime is nil")
	}
	lock.Lock()
	defer lock.Unlock()
	tokenCache[t.token] = expiretime.Unix()
	return nil
}

func (t *K8sToken) IsCacheToken() bool {
	unixtime, ok := tokenCache[t.token]
	if !ok {
		return false
	}
	if unixtime < time.Now().Unix() {
		lock.Lock()
		defer lock.Unlock()
		delete(tokenCache, t.token)
		return false
	}
	return ok
}

func (t *K8sToken) GetSaNameOld() (string, error) {
	sa, _ := getTokenSaName(t.token)
	if sa == "" {
		return "", errors.New("token中没有找到serviceaccount")
	}
	return sa, nil
}

func (t *K8sToken) GetUserName() (string, error) {
	s, err := t.GetAudience()
	if err != nil {
		return "", err
	}
	if len(s) == 0 {
		return "", errors.New("用户名为空")
	}
	return s[0], nil
}

func (t *K8sToken) GetAudience() (jwtv5.ClaimStrings, error) {
	data, err := t.parseClaims()
	if err != nil {
		return nil, err
	}

	aud, ok := data["aud"]
	if !ok {
		return nil, errors.New("token中无audience")
	}
	switch v := aud.(type) {
	case []interface{}:
		result := make(jwtv5.ClaimStrings, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				result[i] = s
			}
		}
		return result, nil
	case string:
		return jwtv5.ClaimStrings{v}, nil
	default:
		return nil, errors.New("audience格式错误")
	}
}

func (t *K8sToken) GetExpireTime() (*jwtv5.NumericDate, error) {
	data, err := t.parseClaims()
	if err != nil {
		return nil, err
	}
	expireData, ok := data["exp"].(float64)
	if !ok || expireData == 0 {
		return nil, nil
	}
	exp := jwtv5.NewNumericDate(time.Unix(int64(expireData), 0))
	return exp, nil
}

// u.Name,
/*
func (u *k3kUser) GetTokenAud(cvmName string) []string {
	return []string{
		u.Name,
		u.GetRole(),
		u.Labels[W7_CONSOLE_ID],
		cvmName,
		u.GetK3kNamespace(),
		"https://kubernetes.default.svc.cluster.local",
		"k3s",
	}
}
*/
func (t *K8sToken) Role() string {
	s, err := t.GetAudience()
	if err != nil {
		return "normal"
	}
	return s[1]
}

func (t *K8sToken) GetRole() string {
	aud, err := t.GetAudience()
	if err != nil {
		return "normal"
	}
	if len(aud) > 1 {
		return aud[1]
	}
	return "normal"
}

func (t *K8sToken) IsFounder() bool {
	return t.GetRole() == "founder"
}

func (t *K8sToken) GetNamespace() string {
	aud, err := t.GetAudience()
	if err != nil {
		return ""
	}
	return aud[4]
}

func getTokenSaName(token string) (string, *jwtv5.NumericDate) {
	data := jwtv5.MapClaims{}
	jwtToken, _, err := jwtv5.NewParser().ParseUnverified(token, data)
	if err != nil {
		return "", nil
	}

	expireData, _ := jwtToken.Claims.GetExpirationTime()

	saName, ok := data["kubernetes.io/serviceaccount/service-account.name"]
	if ok {
		//获取过期时间

		return saName.(string), expireData
	}

	kubernetesIO, ok := data["kubernetes.io"].(map[string]interface{})
	if !ok {
		return "", expireData
	}

	serviceaccount, ok := kubernetesIO["serviceaccount"].(map[string]interface{})
	if !ok {
		return "", expireData
	}

	serviceaccountName, ok := serviceaccount["name"].(string)
	if !ok {
		return "", expireData
	}
	return serviceaccountName, expireData
}

func (t *K8sToken) GetToken() string {
	return t.token
}
