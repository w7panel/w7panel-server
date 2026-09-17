package controller

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	loginconfig "github.com/w7panel/w7panel/common/service/k8s/loginconfig"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"golang.org/x/oauth2"
)

const oidcLoginStateCookie = "w7panel_oidc_login_state"

type oidcLoginState struct {
	nonce, verifier string
	expiresAt       time.Time
}

var oidcLoginStates = struct {
	sync.Mutex
	values map[string]oidcLoginState
}{values: map[string]oidcLoginState{}}

type LoginOIDC struct{ controller.Abstract }

func (LoginOIDC) Start(ctx *gin.Context) {
	if !helper.IsChildAgent() {
		oidcLoginError(ctx, errors.New("OIDC 登录仅支持子集群"))
		return
	}
	relyingParty, err := oidcLoginProvider(ctx, "")
	if err != nil {
		oidcLoginError(ctx, err)
		return
	}
	state, err := oidcRandomValue()
	if err != nil {
		oidcLoginError(ctx, err)
		return
	}
	nonce, err := oidcRandomValue()
	if err != nil {
		oidcLoginError(ctx, err)
		return
	}
	verifier, err := oidcRandomValue()
	if err != nil {
		oidcLoginError(ctx, err)
		return
	}
	oidcLoginStates.Lock()
	for key, item := range oidcLoginStates.values {
		if time.Now().After(item.expiresAt) {
			delete(oidcLoginStates.values, key)
		}
	}
	oidcLoginStates.values[state] = oidcLoginState{nonce: nonce, verifier: verifier, expiresAt: time.Now().Add(5 * time.Minute)}
	oidcLoginStates.Unlock()
	ctx.SetCookie(oidcLoginStateCookie, state, 300, "/", "", oidcSecure(ctx), true)
	ctx.Redirect(http.StatusFound, rp.AuthURL(state, relyingParty,
		rp.WithCodeChallenge(zitadeloidc.NewSHACodeChallenge(verifier)),
		func() []oauth2.AuthCodeOption { return []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce)} },
	))
}

func (LoginOIDC) Callback(ctx *gin.Context) {
	if !helper.IsChildAgent() {
		oidcLoginError(ctx, errors.New("OIDC 登录仅支持子集群"))
		return
	}
	params := struct {
		Code  string `json:"code" binding:"required"`
		State string `json:"state" binding:"required"`
		Error string `json:"error"`
	}{}
	if err := ctx.ShouldBindJSON(&params); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "error": "OIDC 回调参数无效"})
		return
	}
	state, err := ctx.Cookie(oidcLoginStateCookie)
	if err != nil || state == "" || state != params.State {
		ctx.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "error": "OIDC 登录状态无效或已过期"})
		return
	}
	oidcLoginStates.Lock()
	loginState, ok := oidcLoginStates.values[state]
	delete(oidcLoginStates.values, state)
	oidcLoginStates.Unlock()
	ctx.SetCookie(oidcLoginStateCookie, "", -1, "/", "", oidcSecure(ctx), true)
	if !ok || time.Now().After(loginState.expiresAt) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "error": "OIDC 登录状态无效或已过期"})
		return
	}
	if params.Error != "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "error": params.Error})
		return
	}
	relyingParty, err := oidcLoginProvider(ctx, loginState.nonce)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "error": err.Error()})
		return
	}
	tokens, err := rp.CodeExchange[*zitadeloidc.IDTokenClaims](ctx.Request.Context(), params.Code, relyingParty, rp.WithCodeVerifier(loginState.verifier))
	if err != nil || tokens.IDTokenClaims == nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "error": "OIDC 授权码或身份令牌校验失败"})
		return
	}
	username := strings.TrimSpace(tokens.IDTokenClaims.PreferredUsername)
	if username == "" {
		username = strings.TrimSpace(tokens.IDTokenClaims.Subject)
	}
	// A founder authenticates with the root OIDC provider but is not copied to
	// every child panel.  On a child panel the cluster itself is the delegated
	// identity, so bind that login to the tenant user and its normal privilege.
	if helper.IsChildAgent() && oidcFounder(tokens.IDTokenClaims.Claims) {
		if target := strings.TrimSpace(os.Getenv("K3K_NAME")); target != "" {
			username = target
		}
	}
	if username == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"code": http.StatusUnauthorized, "error": "OIDC 身份令牌缺少用户标识"})
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	user, err := userservice.Get(ctx.Request.Context(), sdk, username)
	if err != nil {
		ctx.JSON(http.StatusForbidden, gin.H{"code": http.StatusForbidden, "error": "OIDC 用户未在当前集群创建"})
		return
	}
	Auth{}.dologinUser(sdk, user, ctx, "oidc", "")
}

func oidcFounder(claims map[string]any) bool {
	value, ok := claims["is_founder"]
	if !ok {
		return false
	}
	founder, ok := value.(bool)
	return ok && founder
}

func oidcLoginProvider(ctx *gin.Context, nonce string) (rp.RelyingParty, error) {
	config, err := loginconfig.Get(ctx.Request.Context(), k8s.NewK8sClient().Sdk)
	if err != nil {
		return nil, errors.New("OIDC 登录配置不存在")
	}
	provider, ok := loginconfig.Provider(config, "oidc")
	if !ok || !provider.Enabled || strings.TrimSpace(provider.DiscoveryURL) == "" || strings.TrimSpace(provider.ClientID) == "" {
		return nil, errors.New("OIDC 登录未启用或配置不完整")
	}
	issuer := strings.TrimSuffix(strings.TrimSpace(provider.DiscoveryURL), "/.well-known/openid-configuration")
	if issuer == provider.DiscoveryURL {
		return nil, errors.New("OIDC Discovery URL 格式不正确")
	}
	scopes := provider.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile"}
	}
	options := []rp.Option{rp.WithCustomDiscoveryUrl(provider.DiscoveryURL)}
	if nonce != "" {
		options = append(options, rp.WithVerifierOpts(rp.WithNonce(func(_ context.Context) string { return nonce })))
	}
	relyingParty, err := rp.NewRelyingPartyOIDC(ctx.Request.Context(), issuer, provider.ClientID, "", oidcCallbackURL(ctx), scopes, options...)
	return relyingParty, err
}

func oidcCallbackURL(ctx *gin.Context) string {
	scheme := "http"
	if oidcSecure(ctx) {
		scheme = "https"
	}
	return scheme + "://" + ctx.Request.Host + "/login/oidc/callback"
}
func oidcSecure(ctx *gin.Context) bool {
	return ctx.Request.TLS != nil || strings.EqualFold(ctx.GetHeader("X-Forwarded-Proto"), "https")
}
func oidcRandomValue() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
func oidcLoginError(ctx *gin.Context, err error) {
	ctx.Redirect(http.StatusFound, "/login?oidc_error="+url.QueryEscape(err.Error()))
}
