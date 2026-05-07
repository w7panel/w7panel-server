package oidc

import (
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
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
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

type AuthorizationCode struct {
	Code              string
	ClientID          string
	RedirectURI       string
	Username          string
	Scopes            []string
	Nonce             string
	CodeChallenge     string
	CodeChallengeMode string
	ExpiresAt         time.Time
}

type Session struct {
	ID        string
	Username  string
	ExpiresAt time.Time
}

type UserClaims struct {
	Subject           string   `json:"sub"`
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Name              string   `json:"name,omitempty"`
	Groups            []string `json:"groups,omitempty"`
	Namespace         string   `json:"namespace,omitempty"`
}

type AccessTokenClaims struct {
	Username string   `json:"username"`
	Scopes   []string `json:"scopes,omitempty"`
	TokenUse string   `json:"token_use"`
	ClientID string   `json:"client_id"`
	jwt.RegisteredClaims
}

type IDTokenClaims struct {
	PreferredUsername string   `json:"preferred_username,omitempty"`
	Name              string   `json:"name,omitempty"`
	Groups            []string `json:"groups,omitempty"`
	Namespace         string   `json:"namespace,omitempty"`
	Nonce             string   `json:"nonce,omitempty"`
	jwt.RegisteredClaims
}

type RefreshTokenClaims struct {
	Username string   `json:"username"`
	Scopes   []string `json:"scopes,omitempty"`
	TokenUse string   `json:"token_use"`
	ClientID string   `json:"client_id"`
	jwt.RegisteredClaims
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
	config    Config
	private   *rsa.PrivateKey
	publicJWK jose.JSONWebKey
	kid       string
	clientsMu sync.RWMutex
	clients   map[string]Client
	codesMu   sync.Mutex
	codes     map[string]AuthorizationCode
	sessionMu sync.Mutex
	sessions  map[string]Session
}

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
	pub.KeyID = base64.RawURLEncoding.EncodeToString(kid)

	server := &Server{
		config:    cfg,
		private:   privateKey,
		publicJWK: pub,
		kid:       pub.KeyID,
		clients:   make(map[string]Client),
		codes:     make(map[string]AuthorizationCode),
		sessions:  make(map[string]Session),
	}

	for _, client := range cfg.Clients {
		if client.ClientID == "" {
			return nil, errors.New("oidc client_id is required")
		}
		if len(client.RedirectURIs) == 0 {
			return nil, fmt.Errorf("oidc client %s redirect_uris is required", client.ClientID)
		}
		mode := client.TokenEndpointAuthMode
		if mode == "" {
			if client.ClientSecret == "" {
				mode = "none"
			} else {
				mode = "client_secret_post"
			}
		}
		if client.Scopes == nil {
			client.Scopes = []string{"openid", "profile"}
		}
		server.clients[client.ClientID] = Client{
			Name:                  client.ClientID,
			ClientID:              client.ClientID,
			ClientSecret:          client.ClientSecret,
			RedirectURIs:          client.RedirectURIs,
			Scopes:                normalizeScopes(client.Scopes),
			RequirePKCE:           client.RequirePKCE || client.ClientSecret == "",
			TokenEndpointAuthMode: mode,
			IsDynamic:             false,
		}
	}

	if err := server.loadDynamicClients(); err != nil {
		return nil, err
	}

	return server, nil
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

func (s *Server) Enabled() bool {
	return s != nil && s.config.Enabled
}

func (s *Server) Issuer(r *http.Request) string {
	if s.config.Issuer != "" {
		return strings.TrimRight(s.config.Issuer, "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = strings.TrimSpace(strings.Split(forwardedHost, ",")[0])
	}
	return fmt.Sprintf("%s://%s/panel-api/v1/oidc", scheme, host)
}

func (s *Server) Discovery(r *http.Request) map[string]any {
	issuer := s.Issuer(r)
	result := map[string]any{
		"issuer":                                issuer,
		"authorization_endpoint":                issuer + "/authorize",
		"token_endpoint":                        issuer + "/token",
		"userinfo_endpoint":                     issuer + "/userinfo",
		"jwks_uri":                              issuer + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "offline_access"},
		"claims_supported":                      []string{"sub", "preferred_username", "name", "groups", "namespace"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic", "none"},
		"code_challenge_methods_supported":      []string{"S256"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
	}
	if s.config.RegistrationEnabled {
		result["registration_endpoint"] = issuer + "/register"
	}
	return result
}

func (s *Server) JWKS() map[string]any {
	return map[string]any{
		"keys": []jose.JSONWebKey{s.publicJWK.Public()},
	}
}

func (s *Server) FindClient(clientID string) (Client, bool) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	client, ok := s.clients[clientID]
	return client, ok
}

func (s *Server) ValidateRedirectURI(client Client, redirectURI string) bool {
	return slices.Contains(client.RedirectURIs, redirectURI)
}

func (s *Server) FilterScopes(client Client, requested []string) []string {
	allowed := make([]string, 0, len(requested))
	for _, scope := range requested {
		if scope == "" {
			continue
		}
		if slices.Contains(client.Scopes, scope) && !slices.Contains(allowed, scope) {
			allowed = append(allowed, scope)
		}
	}
	if !slices.Contains(allowed, "openid") && slices.Contains(client.Scopes, "openid") {
		allowed = append([]string{"openid"}, allowed...)
	}
	return allowed
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

func (s *Server) CreateAuthorizationCode(code AuthorizationCode) AuthorizationCode {
	s.codesMu.Lock()
	defer s.codesMu.Unlock()
	s.pruneCodesLocked()
	code.Code = randomToken(32)
	code.ExpiresAt = time.Now().Add(s.config.CodeTTL)
	s.codes[code.Code] = code
	return code
}

func (s *Server) ConsumeAuthorizationCode(value string) (AuthorizationCode, bool) {
	s.codesMu.Lock()
	defer s.codesMu.Unlock()
	code, ok := s.codes[value]
	if !ok {
		return AuthorizationCode{}, false
	}
	delete(s.codes, value)
	if time.Now().After(code.ExpiresAt) {
		return AuthorizationCode{}, false
	}
	return code, true
}

func (s *Server) CreateTokenPair(issuer string, client Client, username string, scopes []string, nonce string) (map[string]any, error) {
	now := time.Now()
	userClaims := s.BuildUserClaims(username)
	accessClaims := AccessTokenClaims{
		Username: username,
		Scopes:   scopes,
		TokenUse: "access_token",
		ClientID: client.ClientID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userClaims.Subject,
			Audience:  jwt.ClaimStrings{client.ClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        randomToken(24),
		},
	}
	idClaims := IDTokenClaims{
		PreferredUsername: userClaims.PreferredUsername,
		Name:              userClaims.Name,
		Groups:            userClaims.Groups,
		Namespace:         userClaims.Namespace,
		Nonce:             nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userClaims.Subject,
			Audience:  jwt.ClaimStrings{client.ClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        randomToken(24),
		},
	}

	accessToken, err := s.signJWT(accessClaims)
	if err != nil {
		return nil, err
	}
	idToken, err := s.signJWT(idClaims)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"access_token": accessToken,
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   int(s.config.AccessTokenTTL.Seconds()),
		"scope":        strings.Join(scopes, " "),
	}
	if slices.Contains(scopes, "offline_access") {
		refreshToken, err := s.CreateRefreshToken(issuer, client, username, scopes)
		if err != nil {
			return nil, err
		}
		result["refresh_token"] = refreshToken
	}
	return result, nil
}

func (s *Server) ParseAccessToken(token string, issuer string) (*AccessTokenClaims, error) {
	claims := &AccessTokenClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return &s.private.PublicKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.Issuer != issuer {
		return nil, errors.New("invalid issuer")
	}
	if claims.TokenUse != "access_token" {
		return nil, errors.New("invalid token_use")
	}
	return claims, nil
}

func (s *Server) ParseRefreshToken(token string, issuer string) (*RefreshTokenClaims, error) {
	claims := &RefreshTokenClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return &s.private.PublicKey, nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, errors.New("invalid refresh token")
	}
	if claims.Issuer != issuer {
		return nil, errors.New("invalid issuer")
	}
	if claims.TokenUse != "refresh_token" {
		return nil, errors.New("invalid token_use")
	}
	return claims, nil
}

func (s *Server) BuildUserClaims(username string) UserClaims {
	claims := UserClaims{
		Subject:           username,
		PreferredUsername: username,
		Name:              username,
	}
	return claims
}

func (s *Server) VerifyPKCE(codeVerifier string, codeChallenge string) bool {
	if codeChallenge == "" {
		return true
	}
	sum := sha256.Sum256([]byte(codeVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:]) == codeChallenge
}

func (s *Server) AuthenticateClient(r *http.Request) (Client, error) {
	_ = r.ParseForm()
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok {
		clientID = r.PostForm.Get("client_id")
		clientSecret = r.PostForm.Get("client_secret")
	}
	client, exists := s.FindClient(clientID)
	if !exists {
		return Client{}, errors.New("invalid_client")
	}
	switch client.TokenEndpointAuthMode {
	case "none":
		if clientSecret != "" {
			return Client{}, errors.New("invalid_client")
		}
	case "client_secret_basic", "client_secret_post":
		if client.ClientSecret == "" || clientSecret != client.ClientSecret {
			return Client{}, errors.New("invalid_client")
		}
	default:
		return Client{}, errors.New("invalid_client")
	}
	return client, nil
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
	if len(req.GrantTypes) > 0 && !slices.Equal(req.GrantTypes, []string{"authorization_code"}) &&
		!slices.Equal(req.GrantTypes, []string{"authorization_code", "refresh_token"}) &&
		!slices.Equal(req.GrantTypes, []string{"refresh_token", "authorization_code"}) {
		return nil, errors.New("only authorization_code and refresh_token grant_types are supported")
	}
	if len(req.ResponseTypes) > 0 && !slices.Equal(req.ResponseTypes, []string{"code"}) {
		return nil, errors.New("only code response_type is supported")
	}
	mode := req.TokenEndpointAuthMode
	if mode == "" {
		mode = "client_secret_basic"
	}
	if mode != "client_secret_basic" && mode != "client_secret_post" && mode != "none" {
		return nil, errors.New("unsupported token_endpoint_auth_method")
	}

	clientID := "oidc_" + randomToken(16)
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
		ClientID:              normalizeClientID(clientID),
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
	client, ok := s.FindClient(clientID)
	if !ok || !client.IsDynamic {
		return nil, errors.New("client not found")
	}
	return s.clientToResponse(client), nil
}

func (s *Server) UpdateDynamicClient(clientID string, req DynamicClientRequest) (*DynamicClientResponse, error) {
	client, ok := s.FindClient(clientID)
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
	client, ok := s.FindClient(clientID)
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

func (s *Server) signJWT(claims jwt.Claims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	return token.SignedString(s.private)
}

func (s *Server) CreateRefreshToken(issuer string, client Client, username string, scopes []string) (string, error) {
	now := time.Now()
	claims := RefreshTokenClaims{
		Username: username,
		Scopes:   scopes,
		TokenUse: "refresh_token",
		ClientID: client.ClientID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   username,
			Audience:  jwt.ClaimStrings{client.ClientID},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshTokenTTL)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        randomToken(24),
		},
	}
	return s.signJWT(claims)
}

func (s *Server) NewTokenPairFromRefreshToken(issuer string, client Client, claims *RefreshTokenClaims) (map[string]any, error) {
	result, err := s.CreateTokenPair(issuer, client, claims.Username, claims.Scopes, "")
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) loadDynamicClients() error {
	sdk := k8s.NewK8sClient().Sdk
	secrets, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).List(sdk.Ctx, metav1.ListOptions{
		LabelSelector: "w7.cc/oidc-client=true",
	})
	if err != nil {
		return err
	}
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()
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
	createdAt := secret.CreationTimestamp.Time
	return Client{
		Name:                  string(secret.Data["client_name"]),
		ClientID:              secret.Name,
		ClientSecret:          string(secret.Data["client_secret"]),
		RedirectURIs:          splitLines(string(secret.Data["redirect_uris"])),
		Scopes:                normalizeScopes(strings.Fields(string(secret.Data["scopes"]))),
		RequirePKCE:           string(secret.Data["require_pkce"]) == "true",
		TokenEndpointAuthMode: string(secret.Data["token_endpoint_auth_method"]),
		IsDynamic:             secret.Labels["w7.cc/oidc-client"] == "true",
		CreatedAt:             createdAt,
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

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, strings.TrimSpace(line))
		}
	}
	return result
}

func normalizeScopes(scopes []string) []string {
	allowed := []string{"openid", "profile", "offline_access"}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if slices.Contains(allowed, scope) && !slices.Contains(result, scope) {
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

func (s *Server) pruneCodesLocked() {
	now := time.Now()
	for code, data := range s.codes {
		if now.After(data.ExpiresAt) {
			delete(s.codes, code)
		}
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

func randomToken(byteLen int) string {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
