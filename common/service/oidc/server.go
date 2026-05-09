package oidc

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultAccessTokenTTL  = time.Hour
	defaultRefreshTokenTTL = 30 * 24 * time.Hour
	defaultCodeTTL         = 5 * time.Minute
	defaultSessionTTL      = 12 * time.Hour
	sessionCookieName      = "w7panel_oidc_session"
	authRequestIDQuery     = "authRequestID"
)

type Config struct {
	Enabled                 bool           `mapstructure:"enabled"`
	Issuer                  string         `mapstructure:"issuer"`
	CookieDomain            string         `mapstructure:"cookie_domain"`
	CookieSecure            bool           `mapstructure:"cookie_secure"`
	SigningKeyPEM           string         `mapstructure:"signing_key_pem"`
	AccessTokenTTL          time.Duration  `mapstructure:"access_token_ttl"`
	RefreshTokenTTL         time.Duration  `mapstructure:"refresh_token_ttl"`
	CodeTTL                 time.Duration  `mapstructure:"code_ttl"`
	SessionTTL              time.Duration  `mapstructure:"session_ttl"`
	RegistrationEnabled     bool           `mapstructure:"registration_enabled"`
	RegistrationAccessToken string         `mapstructure:"registration_access_token"`
	Clients                 []ClientConfig `mapstructure:"clients"`
}

type ClientConfig struct {
	ClientID              string   `mapstructure:"client_id"`
	ClientSecret          string   `mapstructure:"client_secret"`
	RedirectURIs          []string `mapstructure:"redirect_uris"`
	Scopes                []string `mapstructure:"scopes"`
	RequirePKCE           bool     `mapstructure:"require_pkce"`
	TokenEndpointAuthMode string   `mapstructure:"token_endpoint_auth_method"`
}

type Client struct {
	Name                  string
	ClientID              string
	ClientSecret          string
	RedirectURIs          []string
	Scopes                []string
	RequirePKCE           bool
	TokenEndpointAuthMode string
	IsDynamic             bool
	CreatedAt             time.Time
}

type Session struct {
	ID        string
	Username  string
	ExpiresAt time.Time
}

type DynamicClientRequest struct {
	RedirectURIs          []string `json:"redirect_uris"`
	TokenEndpointAuthMode string   `json:"token_endpoint_auth_method"`
	GrantTypes            []string `json:"grant_types"`
	ResponseTypes         []string `json:"response_types"`
	Scope                 string   `json:"scope"`
	ClientName            string   `json:"client_name"`
	RequirePKCE           *bool    `json:"require_pkce"`
}

type DynamicClientResponse struct {
	ClientID              string   `json:"client_id"`
	ClientSecret          string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt      int64    `json:"client_id_issued_at"`
	ClientSecretExpiresAt int64    `json:"client_secret_expires_at"`
	RedirectURIs          []string `json:"redirect_uris"`
	TokenEndpointAuthMode string   `json:"token_endpoint_auth_method"`
	GrantTypes            []string `json:"grant_types"`
	ResponseTypes         []string `json:"response_types"`
	Scope                 string   `json:"scope"`
	ClientName            string   `json:"client_name,omitempty"`
	RequirePKCE           bool     `json:"require_pkce"`
	RegistrationClientURI string   `json:"registration_client_uri,omitempty"`
}

type Server struct {
	config Config

	private *rsa.PrivateKey
	kid     string

	clientsMu sync.RWMutex
	clients   map[string]Client

	authReqMu    sync.Mutex
	authRequests map[string]*authRequest
	authCodes    map[string]string

	tokenMu       sync.Mutex
	accessTokens  map[string]*accessToken
	refreshTokens map[string]*refreshToken

	sessionMu sync.Mutex
	sessions  map[string]Session

	provider op.OpenIDProvider
	legacy   *op.LegacyServer
	handler  http.Handler
}

type accessToken struct {
	ID            string
	ApplicationID string
	RefreshToken  string
	Subject       string
	Audience      []string
	Scopes        []string
	Expiration    time.Time
}

type refreshToken struct {
	Token         string
	ApplicationID string
	Subject       string
	Audience      []string
	Scopes        []string
	AuthTime      time.Time
	AMR           []string
	Expiration    time.Time
	AccessTokenID string
}

type authRequest struct {
	ID            string
	CreationDate  time.Time
	ApplicationID string
	CallbackURI   string
	TransferState string
	Prompt        []string
	UserID        string
	Scopes        []string
	ResponseType  zitadeloidc.ResponseType
	ResponseMode  zitadeloidc.ResponseMode
	Nonce         string
	CodeChallenge *zitadeloidc.CodeChallenge

	done     bool
	authTime time.Time
}

type refreshTokenRequest struct {
	token *refreshToken
}

type signingKey struct {
	id        string
	algorithm jose.SignatureAlgorithm
	key       *rsa.PrivateKey
}

type publicKey struct {
	signingKey
}

type contextKey string

const userContextKey contextKey = "oidc-user"

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
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}

	privateKey, err := parseOrGenerateKey(cfg.SigningKeyPEM)
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
		clients:       make(map[string]Client),
		authRequests:  make(map[string]*authRequest),
		authCodes:     make(map[string]string),
		accessTokens:  make(map[string]*accessToken),
		refreshTokens: make(map[string]*refreshToken),
		sessions:      make(map[string]Session),
	}

	for _, client := range cfg.Clients {
		if client.ClientID == "" {
			return nil, errors.New("oidc client_id is required")
		}
		if len(client.RedirectURIs) == 0 {
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
			Scopes:                scopes,
			RequirePKCE:           client.RequirePKCE || client.ClientSecret == "",
			TokenEndpointAuthMode: mode,
			CreatedAt:             time.Now(),
		}
	}
	if err := s.loadDynamicClients(); err != nil {
		return nil, err
	}
	if err := s.initProvider(); err != nil {
		return nil, err
	}
	return s, nil
}

func parseOrGenerateKey(pemValue string) (*rsa.PrivateKey, error) {
	if strings.TrimSpace(pemValue) == "" {
		return rsa.GenerateKey(rand.Reader, 2048)
	}
	block, _ := pem.Decode([]byte(pemValue))
	if block == nil {
		return nil, errors.New("invalid oidc.signing_key_pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("oidc signing key must be RSA private key")
	}
	return key, nil
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
	issuerBuilder := op.IssuerFromForwardedOrHost("/panel-api/v1/oidc")
	if s.config.Issuer != "" {
		issuerBuilder = op.StaticIssuer(strings.TrimRight(s.config.Issuer, "/"))
	}
	provider, err := op.NewProvider(
		opConfig,
		s,
		issuerBuilder,
		op.WithAllowInsecure(),
		op.WithCustomAuthEndpoint(op.NewEndpoint("authorize")),
		op.WithCustomTokenEndpoint(op.NewEndpoint("token")),
		op.WithCustomUserinfoEndpoint(op.NewEndpoint("userinfo")),
		op.WithCustomKeysEndpoint(op.NewEndpoint("jwks")),
	)
	if err != nil {
		return err
	}
	endpoints := op.Endpoints{
		Authorization: op.NewEndpoint("authorize"),
		Token:         op.NewEndpoint("token"),
		Userinfo:      op.NewEndpoint("userinfo"),
		JwksURI:       op.NewEndpoint("jwks"),
	}
	s.provider = provider
	s.legacy = op.NewLegacyServer(provider, endpoints)
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
	if username := s.currentUsername(r); username != "" {
		r = r.WithContext(context.WithValue(r.Context(), userContextKey, username))
	}
	s.handler.ServeHTTP(w, r)
}

func (s *Server) Issuer(r *http.Request) string {
	if s.config.Issuer != "" {
		return strings.TrimRight(s.config.Issuer, "/")
	}
	return s.provider.IssuerFromRequest(r)
}

func (s *Server) Discovery(r *http.Request) map[string]any {
	result := map[string]any{
		"issuer":                                s.Issuer(r),
		"authorization_endpoint":                s.Issuer(r) + "/authorize",
		"token_endpoint":                        s.Issuer(r) + "/token",
		"userinfo_endpoint":                     s.Issuer(r) + "/userinfo",
		"jwks_uri":                              s.Issuer(r) + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "offline_access"},
		"claims_supported":                      []string{"sub", "preferred_username", "name"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
	}
	if s.config.RegistrationEnabled {
		result["registration_endpoint"] = s.Issuer(r) + "/register"
	}
	return result
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

func (s *Server) Login(ctx context.Context, id, username, password string) error {
	if strings.TrimSpace(username) == "" || password == "" {
		return errors.New("用户名和密码不能为空")
	}
	clientSDK := k8s.NewK8sClient()
	if _, err := clientSDK.Login2(username, password, true); err != nil {
		return errors.New("用户名或密码错误")
	}
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return errors.New("授权请求不存在或已过期")
	}
	req.UserID = username
	req.done = true
	req.authTime = time.Now()
	return nil
}

func (s *Server) CompleteAuthRequest(id, username string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return errors.New("授权请求不存在或已过期")
	}
	req.UserID = username
	req.done = true
	if req.authTime.IsZero() {
		req.authTime = time.Now()
	}
	return nil
}

func (s *Server) CallbackURL(ctx context.Context, id string) string {
	return s.legacy.AuthCallbackURL()(ctx, id)
}

func (s *Server) CreateSession(username string) Session {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.pruneSessionsLocked()
	session := Session{
		ID:        randomToken(32),
		Username:  username,
		ExpiresAt: time.Now().Add(s.config.SessionTTL),
	}
	s.sessions[session.ID] = session
	return session
}

func (s *Server) FindSession(sessionID string) (Session, bool) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, false
	}
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessions, sessionID)
		return Session{}, false
	}
	return session, true
}

func (s *Server) SetSessionCookie(w http.ResponseWriter, r *http.Request, session Session) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.config.CookieSecure || r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		Expires:  session.ExpiresAt,
	}
	if s.config.CookieDomain != "" {
		cookie.Domain = s.config.CookieDomain
	}
	http.SetCookie(w, cookie)
}

func (s *Server) GetSessionID(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (s *Server) currentUsername(r *http.Request) string {
	sessionID := s.GetSessionID(r)
	if sessionID == "" {
		return ""
	}
	session, ok := s.FindSession(sessionID)
	if !ok {
		return ""
	}
	return session.Username
}

func (s *Server) RegisterEnabled() bool {
	return s.config.RegistrationEnabled
}

func (s *Server) ValidateRegistrationAccessToken(token string) bool {
	if !s.config.RegistrationEnabled {
		return false
	}
	if s.config.RegistrationAccessToken == "" {
		return true
	}
	return token == s.config.RegistrationAccessToken
}

func (s *Server) RegisterDynamicClient(req DynamicClientRequest) (*DynamicClientResponse, error) {
	if len(req.RedirectURIs) == 0 {
		return nil, errors.New("redirect_uris is required")
	}
	for _, redirectURI := range req.RedirectURIs {
		if !(strings.HasPrefix(redirectURI, "http://") || strings.HasPrefix(redirectURI, "https://")) {
			return nil, fmt.Errorf("invalid redirect uri: %s", redirectURI)
		}
	}
	if len(req.GrantTypes) > 0 && !sameStrings(req.GrantTypes, []string{"authorization_code"}) &&
		!sameStrings(req.GrantTypes, []string{"authorization_code", "refresh_token"}) {
		return nil, errors.New("only authorization_code and refresh_token grant_types are supported")
	}
	if len(req.ResponseTypes) > 0 && !sameStrings(req.ResponseTypes, []string{"code"}) {
		return nil, errors.New("only code response_type is supported")
	}
	mode := normalizeAuthMethod(req.TokenEndpointAuthMode, "x")
	if mode != "client_secret_basic" && mode != "client_secret_post" && mode != "none" {
		return nil, errors.New("unsupported token_endpoint_auth_method")
	}

	clientID := normalizeClientID("oidc_" + randomToken(16))
	clientSecret := ""
	requirePKCE := mode == "none"
	if req.RequirePKCE != nil {
		requirePKCE = *req.RequirePKCE
	}
	if mode != "none" {
		clientSecret = randomToken(24)
	}
	scopes := normalizeScopes(strings.Fields(req.Scope))
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "offline_access"}
	}
	client := Client{
		Name:                  req.ClientName,
		ClientID:              clientID,
		ClientSecret:          clientSecret,
		RedirectURIs:          req.RedirectURIs,
		Scopes:                scopes,
		RequirePKCE:           requirePKCE,
		TokenEndpointAuthMode: mode,
		IsDynamic:             true,
		CreatedAt:             time.Now(),
	}
	if err := s.saveDynamicClient(client, false); err != nil {
		return nil, err
	}
	s.clientsMu.Lock()
	s.clients[client.ClientID] = client
	s.clientsMu.Unlock()
	return s.clientToResponse(client), nil
}

func (s *Server) GetDynamicClient(clientID string) (*DynamicClientResponse, error) {
	client, ok := s.findClient(clientID)
	if !ok || !client.IsDynamic {
		return nil, errors.New("client not found")
	}
	return s.clientToResponse(client), nil
}

func (s *Server) UpdateDynamicClient(clientID string, req DynamicClientRequest) (*DynamicClientResponse, error) {
	client, ok := s.findClient(clientID)
	if !ok || !client.IsDynamic {
		return nil, errors.New("client not found")
	}
	if len(req.RedirectURIs) > 0 {
		client.RedirectURIs = req.RedirectURIs
	}
	if req.Scope != "" {
		client.Scopes = normalizeScopes(strings.Fields(req.Scope))
	}
	if req.ClientName != "" {
		client.Name = req.ClientName
	}
	if req.RequirePKCE != nil {
		client.RequirePKCE = *req.RequirePKCE
	}
	if req.TokenEndpointAuthMode != "" {
		client.TokenEndpointAuthMode = req.TokenEndpointAuthMode
	}
	if err := s.saveDynamicClient(client, true); err != nil {
		return nil, err
	}
	s.clientsMu.Lock()
	s.clients[client.ClientID] = client
	s.clientsMu.Unlock()
	return s.clientToResponse(client), nil
}

func (s *Server) DeleteDynamicClient(clientID string) error {
	client, ok := s.findClient(clientID)
	if !ok || !client.IsDynamic {
		return errors.New("client not found")
	}
	sdk := k8s.NewK8sClient().Sdk
	err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Delete(sdk.Ctx, clientID, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	s.clientsMu.Lock()
	delete(s.clients, clientID)
	s.clientsMu.Unlock()
	return nil
}

func (s *Server) findClient(clientID string) (Client, bool) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	client, ok := s.clients[clientID]
	return client, ok
}

func (s *Server) CheckUsernamePassword(_ context.Context, username, password, id string) error {
	return s.Login(context.Background(), id, username, password)
}

func (s *Server) CreateAuthRequest(_ context.Context, authReq *zitadeloidc.AuthRequest, userID string) (op.AuthRequest, error) {
	if len(authReq.Prompt) == 1 && authReq.Prompt[0] == zitadeloidc.PromptNone && userID == "" {
		return nil, zitadeloidc.ErrLoginRequired()
	}
	req := &authRequest{
		ID:            uuid.NewString(),
		CreationDate:  time.Now(),
		ApplicationID: authReq.ClientID,
		CallbackURI:   authReq.RedirectURI,
		TransferState: authReq.State,
		Prompt:        append([]string{}, authReq.Prompt...),
		UserID:        userID,
		Scopes:        append([]string{}, authReq.Scopes...),
		ResponseType:  authReq.ResponseType,
		ResponseMode:  authReq.ResponseMode,
		Nonce:         authReq.Nonce,
		done:          userID != "",
	}
	if authReq.CodeChallenge != "" {
		req.CodeChallenge = &zitadeloidc.CodeChallenge{
			Challenge: authReq.CodeChallenge,
			Method:    authReq.CodeChallengeMethod,
		}
	}
	if req.done {
		req.authTime = time.Now()
	}
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	s.pruneAuthRequestsLocked()
	s.authRequests[req.ID] = req
	return req, nil
}

func (s *Server) AuthRequestByID(_ context.Context, id string) (op.AuthRequest, error) {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	req, ok := s.authRequests[id]
	if !ok {
		return nil, errors.New("request not found")
	}
	if req.CreationDate.Add(s.config.CodeTTL).Before(time.Now()) {
		delete(s.authRequests, id)
		return nil, errors.New("request expired")
	}
	return req, nil
}

func (s *Server) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	s.authReqMu.Lock()
	id, ok := s.authCodes[code]
	s.authReqMu.Unlock()
	if !ok {
		return nil, errors.New("code invalid or expired")
	}
	return s.AuthRequestByID(ctx, id)
}

func (s *Server) SaveAuthCode(_ context.Context, id string, code string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	s.authCodes[code] = id
	return nil
}

func (s *Server) DeleteAuthRequest(_ context.Context, id string) error {
	s.authReqMu.Lock()
	defer s.authReqMu.Unlock()
	delete(s.authRequests, id)
	for code, reqID := range s.authCodes {
		if reqID == id {
			delete(s.authCodes, code)
		}
	}
	return nil
}

func (s *Server) CreateAccessToken(_ context.Context, request op.TokenRequest) (string, time.Time, error) {
	clientID := ""
	if req, ok := request.(interface{ GetClientID() string }); ok {
		clientID = req.GetClientID()
	}
	token := &accessToken{
		ID:            uuid.NewString(),
		ApplicationID: clientID,
		Subject:       request.GetSubject(),
		Audience:      append([]string{}, request.GetAudience()...),
		Scopes:        append([]string{}, request.GetScopes()...),
		Expiration:    time.Now().Add(s.config.AccessTokenTTL),
	}
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.pruneTokensLocked()
	s.accessTokens[token.ID] = token
	return token.ID, token.Expiration, nil
}

func (s *Server) CreateAccessAndRefreshTokens(_ context.Context, request op.TokenRequest, currentRefreshToken string) (string, string, time.Time, error) {
	clientID := ""
	authTime := time.Now()
	amr := []string{"pwd"}
	if req, ok := request.(interface{ GetClientID() string }); ok {
		clientID = req.GetClientID()
	}
	if req, ok := request.(interface{ GetAuthTime() time.Time }); ok && !req.GetAuthTime().IsZero() {
		authTime = req.GetAuthTime()
	}
	if req, ok := request.(interface{ GetAMR() []string }); ok && len(req.GetAMR()) > 0 {
		amr = append([]string{}, req.GetAMR()...)
	}

	newRefreshToken := currentRefreshToken
	if newRefreshToken == "" {
		newRefreshToken = randomToken(32)
	} else {
		newRefreshToken = randomToken(32)
	}
	token := &accessToken{
		ID:            uuid.NewString(),
		ApplicationID: clientID,
		RefreshToken:  newRefreshToken,
		Subject:       request.GetSubject(),
		Audience:      append([]string{}, request.GetAudience()...),
		Scopes:        append([]string{}, request.GetScopes()...),
		Expiration:    time.Now().Add(s.config.AccessTokenTTL),
	}
	rt := &refreshToken{
		Token:         newRefreshToken,
		ApplicationID: clientID,
		Subject:       request.GetSubject(),
		Audience:      append([]string{}, request.GetAudience()...),
		Scopes:        append([]string{}, request.GetScopes()...),
		AuthTime:      authTime,
		AMR:           amr,
		Expiration:    time.Now().Add(s.config.RefreshTokenTTL),
		AccessTokenID: token.ID,
	}

	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	s.pruneTokensLocked()
	if currentRefreshToken != "" {
		if current, ok := s.refreshTokens[currentRefreshToken]; !ok || current.Expiration.Before(time.Now()) {
			return "", "", time.Time{}, errors.New("invalid refresh token")
		} else {
			delete(s.refreshTokens, currentRefreshToken)
			delete(s.accessTokens, current.AccessTokenID)
		}
	}
	s.accessTokens[token.ID] = token
	s.refreshTokens[newRefreshToken] = rt
	return token.ID, newRefreshToken, token.Expiration, nil
}

func (s *Server) TokenRequestByRefreshToken(_ context.Context, refreshTokenValue string) (op.RefreshTokenRequest, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	token, ok := s.refreshTokens[refreshTokenValue]
	if !ok || token.Expiration.Before(time.Now()) {
		return nil, errors.New("invalid refresh_token")
	}
	return &refreshTokenRequest{token: token}, nil
}

func (s *Server) TerminateSession(_ context.Context, userID string, clientID string) error {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	for tokenID, token := range s.accessTokens {
		if token.ApplicationID == clientID && token.Subject == userID {
			delete(s.accessTokens, tokenID)
		}
	}
	for tokenValue, token := range s.refreshTokens {
		if token.ApplicationID == clientID && token.Subject == userID {
			delete(s.refreshTokens, tokenValue)
		}
	}
	return nil
}

func (s *Server) RevokeToken(_ context.Context, tokenIDOrToken string, _ string, clientID string) *zitadeloidc.Error {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if token, ok := s.accessTokens[tokenIDOrToken]; ok {
		if token.ApplicationID != clientID {
			return zitadeloidc.ErrInvalidClient().WithDescription("token was not issued for this client")
		}
		delete(s.accessTokens, tokenIDOrToken)
		return nil
	}
	if token, ok := s.refreshTokens[tokenIDOrToken]; ok {
		if token.ApplicationID != clientID {
			return zitadeloidc.ErrInvalidClient().WithDescription("token was not issued for this client")
		}
		delete(s.refreshTokens, tokenIDOrToken)
		delete(s.accessTokens, token.AccessTokenID)
	}
	return nil
}

func (s *Server) GetRefreshTokenInfo(_ context.Context, clientID string, token string) (string, string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	refreshToken, ok := s.refreshTokens[token]
	if !ok || refreshToken.ApplicationID != clientID {
		return "", "", op.ErrInvalidRefreshToken
	}
	return refreshToken.Subject, refreshToken.Token, nil
}

func (s *Server) SigningKey(context.Context) (op.SigningKey, error) {
	return &signingKey{id: s.kid, algorithm: jose.RS256, key: s.private}, nil
}

func (s *Server) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}

func (s *Server) KeySet(context.Context) ([]op.Key, error) {
	return []op.Key{&publicKey{signingKey{id: s.kid, algorithm: jose.RS256, key: s.private}}}, nil
}

func (s *Server) GetClientByClientID(_ context.Context, clientID string) (op.Client, error) {
	client, ok := s.findClient(clientID)
	if !ok {
		return nil, errors.New("client not found")
	}
	return oidcClient{client: client}, nil
}

func (s *Server) AuthorizeClientIDSecret(_ context.Context, clientID, clientSecret string) error {
	client, ok := s.findClient(clientID)
	if !ok {
		return errors.New("client not found")
	}
	switch client.TokenEndpointAuthMode {
	case "none":
		if clientSecret != "" {
			return errors.New("invalid secret")
		}
	case "client_secret_basic", "client_secret_post":
		if client.ClientSecret == "" || client.ClientSecret != clientSecret {
			return errors.New("invalid secret")
		}
	default:
		return errors.New("invalid auth method")
	}
	return nil
}

func (s *Server) SetUserinfoFromScopes(context.Context, *zitadeloidc.UserInfo, string, string, []string) error {
	return nil
}

func (s *Server) SetUserinfoFromRequest(_ context.Context, userinfo *zitadeloidc.UserInfo, token op.IDTokenRequest, scopes []string) error {
	return s.setUserinfo(userinfo, token.GetSubject(), scopes)
}

func (s *Server) SetUserinfoFromToken(_ context.Context, userinfo *zitadeloidc.UserInfo, tokenID, subject, _ string) error {
	s.tokenMu.Lock()
	token, ok := s.accessTokens[tokenID]
	s.tokenMu.Unlock()
	if !ok || token.Expiration.Before(time.Now()) {
		return errors.New("token is invalid or has expired")
	}
	return s.setUserinfo(userinfo, subject, token.Scopes)
}

func (s *Server) SetIntrospectionFromToken(_ context.Context, introspection *zitadeloidc.IntrospectionResponse, tokenID, subject, clientID string) error {
	s.tokenMu.Lock()
	token, ok := s.accessTokens[tokenID]
	s.tokenMu.Unlock()
	if !ok || token.Expiration.Before(time.Now()) {
		return errors.New("token is invalid or expired")
	}
	validAudience := false
	for _, aud := range token.Audience {
		if aud == clientID {
			validAudience = true
			break
		}
	}
	if !validAudience {
		return errors.New("token is not valid for this client")
	}
	introspection.Expiration = zitadeloidc.FromTime(token.Expiration)
	introspection.Scope = token.Scopes
	introspection.ClientID = token.ApplicationID
	userInfo := new(zitadeloidc.UserInfo)
	if err := s.setUserinfo(userInfo, subject, token.Scopes); err != nil {
		return err
	}
	introspection.SetUserInfo(userInfo)
	return nil
}

func (s *Server) GetPrivateClaimsFromScopes(_ context.Context, userID, clientID string, scopes []string) (map[string]any, error) {
	claims := map[string]any{
		"username":  userID,
		"client_id": clientID,
		"scopes":    scopes,
	}
	return claims, nil
}

func (s *Server) GetKeyByIDAndClientID(_ context.Context, _ string, _ string) (*jose.JSONWebKey, error) {
	return nil, errors.New("private_key_jwt not supported")
}

func (s *Server) ValidateJWTProfileScopes(_ context.Context, _ string, scopes []string) ([]string, error) {
	return scopes, nil
}

func (s *Server) Health(context.Context) error {
	return nil
}

func (s *Server) setUserinfo(userinfo *zitadeloidc.UserInfo, subject string, scopes []string) error {
	for _, scope := range scopes {
		switch scope {
		case zitadeloidc.ScopeOpenID:
			userinfo.Subject = subject
		case zitadeloidc.ScopeProfile:
			userinfo.PreferredUsername = subject
			userinfo.Name = subject
		}
	}
	if userinfo.Subject == "" {
		userinfo.Subject = subject
	}
	return nil
}

func (r *authRequest) GetID() string  { return r.ID }
func (r *authRequest) GetACR() string { return "" }
func (r *authRequest) GetAMR() []string {
	if r.done {
		return []string{"pwd"}
	}
	return nil
}
func (r *authRequest) GetAudience() []string                        { return []string{r.ApplicationID} }
func (r *authRequest) GetAuthTime() time.Time                       { return r.authTime }
func (r *authRequest) GetClientID() string                          { return r.ApplicationID }
func (r *authRequest) GetCodeChallenge() *zitadeloidc.CodeChallenge { return r.CodeChallenge }
func (r *authRequest) GetNonce() string                             { return r.Nonce }
func (r *authRequest) GetRedirectURI() string                       { return r.CallbackURI }
func (r *authRequest) GetResponseType() zitadeloidc.ResponseType    { return r.ResponseType }
func (r *authRequest) GetResponseMode() zitadeloidc.ResponseMode    { return r.ResponseMode }
func (r *authRequest) GetScopes() []string                          { return r.Scopes }
func (r *authRequest) GetState() string                             { return r.TransferState }
func (r *authRequest) GetSubject() string                           { return r.UserID }
func (r *authRequest) Done() bool                                   { return r.done }
func (r *refreshTokenRequest) GetAMR() []string                     { return r.token.AMR }
func (r *refreshTokenRequest) GetAudience() []string                { return r.token.Audience }
func (r *refreshTokenRequest) GetAuthTime() time.Time               { return r.token.AuthTime }
func (r *refreshTokenRequest) GetClientID() string                  { return r.token.ApplicationID }
func (r *refreshTokenRequest) GetScopes() []string                  { return r.token.Scopes }
func (r *refreshTokenRequest) GetSubject() string                   { return r.token.Subject }
func (r *refreshTokenRequest) SetCurrentScopes(scopes []string)     { r.token.Scopes = scopes }
func (s *signingKey) SignatureAlgorithm() jose.SignatureAlgorithm   { return s.algorithm }
func (s *signingKey) Key() any                                      { return s.key }
func (s *signingKey) ID() string                                    { return s.id }
func (p *publicKey) ID() string                                     { return p.id }
func (p *publicKey) Algorithm() jose.SignatureAlgorithm             { return p.algorithm }
func (p *publicKey) Use() string                                    { return "sig" }
func (p *publicKey) Key() any                                       { return &p.key.PublicKey }

type oidcClient struct {
	client Client
}

func (c oidcClient) GetID() string { return c.client.ClientID }
func (c oidcClient) RedirectURIs() []string {
	return c.client.RedirectURIs
}
func (c oidcClient) PostLogoutRedirectURIs() []string { return nil }
func (c oidcClient) ApplicationType() op.ApplicationType {
	if c.client.ClientSecret == "" {
		return op.ApplicationTypeNative
	}
	return op.ApplicationTypeWeb
}
func (c oidcClient) AuthMethod() zitadeloidc.AuthMethod {
	switch c.client.TokenEndpointAuthMode {
	case "client_secret_post":
		return zitadeloidc.AuthMethodPost
	case "none":
		return zitadeloidc.AuthMethodNone
	default:
		return zitadeloidc.AuthMethodBasic
	}
}
func (c oidcClient) ResponseTypes() []zitadeloidc.ResponseType {
	return []zitadeloidc.ResponseType{zitadeloidc.ResponseTypeCode}
}
func (c oidcClient) GrantTypes() []zitadeloidc.GrantType {
	grants := []zitadeloidc.GrantType{zitadeloidc.GrantTypeCode}
	if containsString(c.client.Scopes, zitadeloidc.ScopeOfflineAccess) {
		grants = append(grants, zitadeloidc.GrantTypeRefreshToken)
	}
	return grants
}
func (c oidcClient) LoginURL(id string) string {
	return "/panel-api/v1/oidc/authorize/login?" + authRequestIDQuery + "=" + id
}
func (c oidcClient) AccessTokenType() op.AccessTokenType { return op.AccessTokenTypeJWT }
func (c oidcClient) IDTokenLifetime() time.Duration      { return time.Hour }
func (c oidcClient) DevMode() bool                       { return true }
func (c oidcClient) RestrictAdditionalIdTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string { return scopes }
}
func (c oidcClient) RestrictAdditionalAccessTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string { return scopes }
}
func (c oidcClient) IsScopeAllowed(scope string) bool {
	return containsString(c.client.Scopes, scope)
}
func (c oidcClient) IDTokenUserinfoClaimsAssertion() bool { return true }
func (c oidcClient) ClockSkew() time.Duration             { return 0 }

func (s *Server) loadDynamicClients() error {
	sdk := k8s.NewK8sClient().Sdk
	secrets, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).List(sdk.Ctx, metav1.ListOptions{
		LabelSelector: "w7.cc/oidc-client=true",
	})
	if err != nil {
		return err
	}
	for _, secret := range secrets.Items {
		client := clientFromSecret(&secret)
		if client.ClientID != "" {
			s.clients[client.ClientID] = client
		}
	}
	return nil
}

func (s *Server) saveDynamicClient(client Client, isUpdate bool) error {
	sdk := k8s.NewK8sClient().Sdk
	secret := secretFromClient(sdk.GetNamespace(), client)
	if isUpdate {
		current, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, client.ClientID, metav1.GetOptions{})
		if err != nil {
			return err
		}
		secret.ResourceVersion = current.ResourceVersion
		_, err = sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Update(sdk.Ctx, secret, metav1.UpdateOptions{})
		return err
	}
	_, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Create(sdk.Ctx, secret, metav1.CreateOptions{})
	return err
}

func clientFromSecret(secret *corev1.Secret) Client {
	return Client{
		Name:                  string(secret.Data["client_name"]),
		ClientID:              secret.Name,
		ClientSecret:          string(secret.Data["client_secret"]),
		RedirectURIs:          splitLines(string(secret.Data["redirect_uris"])),
		Scopes:                normalizeScopes(strings.Fields(string(secret.Data["scopes"]))),
		RequirePKCE:           string(secret.Data["require_pkce"]) == "true",
		TokenEndpointAuthMode: string(secret.Data["token_endpoint_auth_method"]),
		IsDynamic:             secret.Labels["w7.cc/oidc-client"] == "true",
		CreatedAt:             secret.CreationTimestamp.Time,
	}
}

func secretFromClient(namespace string, client Client) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      client.ClientID,
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/oidc-client": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client_name":                []byte(client.Name),
			"client_secret":              []byte(client.ClientSecret),
			"redirect_uris":              []byte(strings.Join(client.RedirectURIs, "\n")),
			"scopes":                     []byte(strings.Join(client.Scopes, " ")),
			"require_pkce":               []byte(fmt.Sprintf("%t", client.RequirePKCE)),
			"token_endpoint_auth_method": []byte(client.TokenEndpointAuthMode),
		},
	}
}

func (s *Server) clientToResponse(client Client) *DynamicClientResponse {
	return &DynamicClientResponse{
		ClientID:              client.ClientID,
		ClientSecret:          client.ClientSecret,
		ClientIDIssuedAt:      client.CreatedAt.Unix(),
		ClientSecretExpiresAt: 0,
		RedirectURIs:          client.RedirectURIs,
		TokenEndpointAuthMode: client.TokenEndpointAuthMode,
		GrantTypes:            []string{"authorization_code", "refresh_token"},
		ResponseTypes:         []string{"code"},
		Scope:                 strings.Join(client.Scopes, " "),
		ClientName:            client.Name,
		RequirePKCE:           client.RequirePKCE,
		RegistrationClientURI: "/panel-api/v1/oidc/register/" + client.ClientID,
	}
}

func (s *Server) pruneSessionsLocked() {
	now := time.Now()
	for key, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, key)
		}
	}
}

func (s *Server) pruneAuthRequestsLocked() {
	now := time.Now()
	for id, req := range s.authRequests {
		if req.CreationDate.Add(s.config.CodeTTL).Before(now) {
			delete(s.authRequests, id)
		}
	}
}

func (s *Server) pruneTokensLocked() {
	now := time.Now()
	for id, token := range s.accessTokens {
		if now.After(token.Expiration) {
			delete(s.accessTokens, id)
		}
	}
	for tokenValue, token := range s.refreshTokens {
		if now.After(token.Expiration) {
			delete(s.refreshTokens, tokenValue)
		}
	}
}

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func normalizeScopes(scopes []string) []string {
	allowed := map[string]struct{}{
		"openid":         {},
		"profile":        {},
		"offline_access": {},
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := allowed[scope]; ok && !containsString(result, scope) {
			result = append(result, scope)
		}
	}
	return result
}

func normalizeClientID(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "-")
	if strings.HasPrefix(value, "oidc_") {
		value = strings.ReplaceAll(value, "oidc_", "oidc-")
	}
	return value
}

func normalizeAuthMethod(mode, secret string) string {
	if mode != "" {
		return mode
	}
	if secret == "" {
		return "none"
	}
	return "client_secret_basic"
}

func randomToken(length int) string {
	size := length
	if size < 16 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:length]
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, item := range left {
		seen[item]++
	}
	for _, item := range right {
		seen[item]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
