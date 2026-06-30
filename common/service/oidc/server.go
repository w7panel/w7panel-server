package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	oidclib "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	defaultAccessTokenTTL  = time.Hour
	defaultRefreshTokenTTL = 30 * 24 * time.Hour
	defaultCodeTTL         = 5 * time.Minute
	authRequestIDQuery     = "authRequestID"
	oidcBasePath           = "/panel-api/v1/oidc"
)

func oidcPath(endpoint string) string {
	if endpoint == "" {
		return oidcBasePath
	}
	return oidcBasePath + "/" + strings.TrimPrefix(endpoint, "/")
}

type Config struct {
	Enabled                     bool           `mapstructure:"enabled"`
	Issuer                      string         `mapstructure:"issuer"`
	SigningKeyPEM               string         `mapstructure:"signing_key_pem"`
	AccessTokenTTL              time.Duration  `mapstructure:"access_token_ttl"`
	RefreshTokenTTL             time.Duration  `mapstructure:"refresh_token_ttl"`
	CodeTTL                     time.Duration  `mapstructure:"code_ttl"`
	InsecureAllowAnyRedirectURI bool           `mapstructure:"insecure_allow_any_redirect_uri"`
	RegistrationEnabled         bool           `mapstructure:"registration_enabled"`
	RegistrationAccessToken     string         `mapstructure:"registration_access_token"`
	Clients                     []ClientConfig `mapstructure:"clients"`
}

type ClientConfig struct {
	ClientID              string   `mapstructure:"client_id"`
	ClientSecret          string   `mapstructure:"client_secret"`
	RedirectURIs          []string `mapstructure:"redirect_uris"`
	AllowAnyRedirectURI   bool     `mapstructure:"allow_any_redirect_uri"`
	Scopes                []string `mapstructure:"scopes"`
	TokenEndpointAuthMode string   `mapstructure:"token_endpoint_auth_method"`
}

type Client struct {
	Name                  string
	ClientID              string
	ClientSecret          string
	RedirectURIs          []string
	AllowAnyRedirectURI   bool
	Scopes                []string
	TokenEndpointAuthMode string
	IsDynamic             bool
	CreatedAt             time.Time
}

type DynamicClientRequest struct {
	RedirectURIs          []string `json:"redirect_uris"`
	AllowAnyRedirectURI   bool     `json:"allow_any_redirect_uri"`
	TokenEndpointAuthMode string   `json:"token_endpoint_auth_method"`
	GrantTypes            []string `json:"grant_types"`
	Scope                 string   `json:"scope"`
	ClientName            string   `json:"client_name"`
}

type DynamicClientResponse struct {
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt      int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt int64    `json:"client_secret_expires_at"`
	RedirectURIs          []string `json:"redirect_uris"`
	AllowAnyRedirectURI   bool     `json:"allow_any_redirect_uri"`
	TokenEndpointAuthMode string   `json:"token_endpoint_auth_method"`
	GrantTypes            []string `json:"grant_types"`
	Scope                 string   `json:"scope"`
	ClientName            string   `json:"client_name,omitempty"`
}

type Server struct {
	config Config

	private *rsa.PrivateKey
	kid     string
	store   dynamicClientStore

	authenticateUser func(context.Context, string, string) (string, error)
	lookupUser       func(context.Context, string) (*k3ktypes.K3kUser, error)

	clientsMu sync.RWMutex
	clients   map[string]Client

	authReqMu    sync.Mutex
	authRequests map[string]*authRequest
	authCodes    map[string]string

	tokenMu       sync.Mutex
	accessTokens  map[string]*accessToken
	refreshTokens map[string]*refreshToken
	accessTokenMu sync.Mutex
	accessTokenBy map[string]*cachedAccessToken

	provider op.OpenIDProvider
	legacy   op.ExtendedLegacyServer
	handler  http.Handler
}

type defaultAccessTokenRequest struct {
	subject  string
	audience []string
	scopes   []string
	clientID string
}

func (r defaultAccessTokenRequest) GetSubject() string    { return r.subject }
func (r defaultAccessTokenRequest) GetAudience() []string { return r.audience }
func (r defaultAccessTokenRequest) GetScopes() []string   { return r.scopes }
func (r defaultAccessTokenRequest) GetClientID() string   { return r.clientID }

var (
	defaultServer *Server
	loadOnce      sync.Once
	loadErr       error
)

func GetServer() (*Server, error) {
	loadOnce.Do(func() {
		cfg := Config{}
		if err := facade.Config.UnmarshalKey("oidc", &cfg); err != nil {
			loadErr = err
			return
		}
		defaultServer, loadErr = NewServer(cfg)
	})
	return defaultServer, loadErr
}

func NewServer(cfg Config) (*Server, error) {
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = defaultAccessTokenTTL
	}
	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = defaultRefreshTokenTTL
	}
	if cfg.CodeTTL <= 0 {
		cfg.CodeTTL = defaultCodeTTL
	}

	privateKey, err := loadOrCreateSigningKey(cfg.SigningKeyPEM)
	if err != nil {
		return nil, err
	}
	pub := jose.JSONWebKey{Key: &privateKey.PublicKey, Algorithm: string(jose.RS256), Use: "sig"}
	kid, err := pub.Thumbprint(crypto.SHA256)
	if err != nil {
		return nil, err
	}

	s := &Server{
		config:        cfg,
		private:       privateKey,
		kid:           base64.RawURLEncoding.EncodeToString(kid),
		store:         newDynamicClientStore(),
		clients:       make(map[string]Client),
		authRequests:  make(map[string]*authRequest),
		authCodes:     make(map[string]string),
		accessTokens:  make(map[string]*accessToken),
		refreshTokens: make(map[string]*refreshToken),
		accessTokenBy: make(map[string]*cachedAccessToken),
		lookupUser:    lookupK3kUser,
	}

	for _, client := range cfg.Clients {
		if client.ClientID == "" {
			return nil, errors.New("oidc client_id is required")
		}
		if len(client.RedirectURIs) == 0 && !client.AllowAnyRedirectURI {
			return nil, fmt.Errorf("oidc client %s redirect_uris is required", client.ClientID)
		}
		mode := normalizeAuthMethod(client.TokenEndpointAuthMode, client.ClientSecret)
		scopes := normalizeScopes(client.Scopes)
		if len(scopes) == 0 {
			scopes = []string{"openid", "profile"}
		}
		s.clients[client.ClientID] = Client{
			Name:                  client.ClientID,
			ClientID:              client.ClientID,
			ClientSecret:          client.ClientSecret,
			RedirectURIs:          client.RedirectURIs,
			AllowAnyRedirectURI:   client.AllowAnyRedirectURI,
			Scopes:                scopes,
			TokenEndpointAuthMode: mode,
			CreatedAt:             time.Now(),
		}
	}
	dynamicClients, err := s.store.Load()
	if err != nil {
		return nil, err
	}
	for _, client := range dynamicClients {
		s.clients[client.ClientID] = client
	}
	if err := s.initProvider(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) initProvider() error {
	cryptoKey := sha256.Sum256(x509.MarshalPKCS1PrivateKey(s.private))
	opConfig := &op.Config{
		CryptoKey:             cryptoKey,
		CryptoKeyId:           s.kid,
		CodeMethodS256:        true,
		AuthMethodPost:        true,
		GrantTypeRefreshToken: true,
	}
	issuerBuilder := op.IssuerFromForwardedOrHost(oidcBasePath)
	if s.config.Issuer != "" {
		issuerBuilder = op.StaticIssuer(strings.TrimRight(s.config.Issuer, "/"))
	}
	provider, err := op.NewProvider(
		opConfig,
		s,
		issuerBuilder,
		op.WithAllowInsecure(),
		op.WithCustomAuthEndpoint(op.NewEndpoint(oidcPath("authorize"))),
		op.WithCustomTokenEndpoint(op.NewEndpoint(oidcPath("token"))),
		op.WithCustomUserinfoEndpoint(op.NewEndpoint(oidcPath("userinfo"))),
		op.WithCustomKeysEndpoint(op.NewEndpoint(oidcPath("jwks"))),
	)
	if err != nil {
		return err
	}
	endpoints := op.Endpoints{
		Authorization: op.NewEndpoint(oidcPath("authorize")),
		Token:         op.NewEndpoint(oidcPath("token")),
		Userinfo:      op.NewEndpoint(oidcPath("userinfo")),
		JwksURI:       op.NewEndpoint(oidcPath("jwks")),
	}
	s.provider = provider
	legacyServer := op.NewLegacyServer(provider, endpoints)
	s.legacy = newLegacyServerAdapters(legacyServer, provider)
	s.handler = op.RegisterLegacyServer(s.legacy, op.AuthorizeCallbackHandler(provider))
	return nil
}

func (s *Server) Enabled() bool {
	return s != nil && s.config.Enabled
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.handler == nil {
		http.NotFound(w, r)
		return
	}
	s.handler.ServeHTTP(w, r)
}

func (s *Server) Issuer(r *http.Request) string {
	if s.config.Issuer != "" {
		return strings.TrimRight(s.config.Issuer, "/")
	}
	return s.provider.IssuerFromRequest(r)
}

func (s *Server) ContextWithIssuer(ctx context.Context, r *http.Request) context.Context {
	if s == nil || r == nil {
		return ctx
	}
	return op.ContextWithIssuer(ctx, s.Issuer(r))
}

func (s *Server) discoveryIssuer(r *http.Request) string {
	issuer := s.Issuer(r)
	parsedIssuer, err := url.Parse(issuer)
	if err != nil {
		return issuer
	}
	parsedIssuer.Path = ""
	parsedIssuer.RawPath = ""
	parsedIssuer.RawQuery = ""
	parsedIssuer.Fragment = ""
	return strings.TrimRight(parsedIssuer.String(), "/")
}

func (s *Server) JWKS() map[string]any {
	return map[string]any{
		"keys": []jose.JSONWebKey{{
			Key:       &s.private.PublicKey,
			KeyID:     s.kid,
			Algorithm: string(jose.RS256),
			Use:       "sig",
		}},
	}
}

func (s *Server) Discovery(ctx context.Context, r *http.Request) (*op.Response, error) {
	data := &struct{}{}
	opReq := &op.Request[struct{}]{
		Method:   r.Method,
		URL:      r.URL,
		Header:   r.Header,
		Form:     r.Form,
		PostForm: r.PostForm,
		Data:     data,
	}
	ls, ok := s.legacy.(*legacyServerAdapters)
	if !ok {
		return nil, fmt.Errorf("legacyServerAdapters 未初始化")
	}
	if ls.LegacyServer == nil {
		return nil, fmt.Errorf("legacyServer 未初始化")
	}
	ctxt := op.ContextWithIssuer(ctx, s.Issuer(r))
	response, err := ls.Discovery(ctxt, opReq)
	if err != nil {
		return nil, err
	}
	resdata := response.Data
	config, ok := resdata.(*oidclib.DiscoveryConfiguration)
	if !ok {
		return nil, fmt.Errorf("response.Data.(*oidclib.DiscoveryConfiguration) 未初始化")
	}
	// ctxt := op.ContextWithIssuer(ctx, s.Issuer(r)) 预设issuer 会导致 其他token 等路径出错 因此改为手动赋值
	config.Issuer = s.Issuer(r)
	config.JwksURI = s.discoveryIssuer(r) + oidcPath("jwks")
	config.AuthorizationEndpoint = s.discoveryIssuer(r) + oidcPath("authorize")
	config.TokenEndpoint = s.discoveryIssuer(r) + oidcPath("token")
	config.UserinfoEndpoint = s.discoveryIssuer(r) + oidcPath("userinfo")
	// config.RevocationEndpoint = s.discoveryIssuer(r) + oidcPath("revoke")
	// config.IntrospectionEndpoint = s.discoveryIssuer(r) + oidcPath("introspect")
	// config.EndSessionEndpoint = s.discoveryIssuer(r) + oidcPath("end_session")
	return response, nil
}

func (s *Server) CreateDefaultAccessToken(ctx context.Context, subject string) (string, error) {
	return s.getOrCreateDefaultAccessToken(ctx, subject)
}
