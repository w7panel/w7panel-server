package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type fakeDynamicClientStore struct {
	loadClients []Client
	createResp  Client
	saveCalls   []saveCall
	createCalls []DynamicClientRequest
	deleteCalls []string
	getCalls    []string
	createErr   error
	getErr      error
	saveErr     error
	deleteErr   error
	loadErr     error
}

type saveCall struct {
	client   Client
	isUpdate bool
}

func (f *fakeDynamicClientStore) Load() ([]Client, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	return append([]Client{}, f.loadClients...), nil
}

func (f *fakeDynamicClientStore) Get(clientID string) (Client, error) {
	if f.getErr != nil {
		return Client{}, f.getErr
	}
	f.getCalls = append(f.getCalls, clientID)
	for _, client := range f.loadClients {
		if client.ClientID == clientID {
			return client, nil
		}
	}
	return Client{}, context.Canceled
}

func (f *fakeDynamicClientStore) Create(req DynamicClientRequest) (Client, error) {
	if f.createErr != nil {
		return Client{}, f.createErr
	}
	f.createCalls = append(f.createCalls, req)
	if f.createResp.ClientID != "" {
		return f.createResp, nil
	}
	mode := normalizeAuthMethod(req.TokenEndpointAuthMode, "x")
	client := Client{
		Name:                  req.ClientName,
		ClientID:              "oidc-test-client",
		RedirectURIs:          append([]string{}, req.RedirectURIs...),
		AllowAnyRedirectURI:   req.AllowAnyRedirectURI,
		Scopes:                normalizeScopes(strings.Fields(req.Scope)),
		TokenEndpointAuthMode: mode,
		IsDynamic:             true,
		CreatedAt:             time.Unix(123, 0),
	}
	if len(client.Scopes) == 0 {
		client.Scopes = []string{"openid", "profile", "offline_access"}
	}
	if mode != "none" {
		client.ClientSecret = "generated-secret"
	}
	f.saveCalls = append(f.saveCalls, saveCall{client: client, isUpdate: false})
	return client, nil
}

func (f *fakeDynamicClientStore) Save(client Client, isUpdate bool) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saveCalls = append(f.saveCalls, saveCall{client: client, isUpdate: isUpdate})
	return nil
}

func (f *fakeDynamicClientStore) Delete(clientID string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleteCalls = append(f.deleteCalls, clientID)
	return nil
}

type testTokenRequest struct {
	subject  string
	audience []string
	scopes   []string
	clientID string
	authTime time.Time
	amr      []string
}

func (r testTokenRequest) GetSubject() string     { return r.subject }
func (r testTokenRequest) GetAudience() []string  { return r.audience }
func (r testTokenRequest) GetScopes() []string    { return r.scopes }
func (r testTokenRequest) GetClientID() string    { return r.clientID }
func (r testTokenRequest) GetAuthTime() time.Time { return r.authTime }
func (r testTokenRequest) GetAMR() []string       { return r.amr }

func newTestServer(store dynamicClientStore) *Server {
	return &Server{
		config: Config{
			AccessTokenTTL:  time.Hour,
			RefreshTokenTTL: 2 * time.Hour,
			CodeTTL:         time.Minute,
		},
		store:         store,
		clients:       map[string]Client{},
		authRequests:  map[string]*authRequest{},
		authCodes:     map[string]string{},
		accessTokens:  map[string]*accessToken{},
		refreshTokens: map[string]*refreshToken{},
		authenticateUser: func(_ context.Context, username, password string) (string, error) {
			if username == "" || password == "" {
				return "", context.Canceled
			}
			return username, nil
		},
	}
}

func TestRegisterDynamicClientStoresAndDefaults(t *testing.T) {
	store := &fakeDynamicClientStore{}
	server := newTestServer(store)

	resp, err := server.RegisterDynamicClient(DynamicClientRequest{
		RedirectURIs:          []string{"https://client.example/callback"},
		TokenEndpointAuthMode: "none",
		ClientName:            "demo",
	})
	if err != nil {
		t.Fatalf("RegisterDynamicClient returned error: %v", err)
	}
	if len(store.saveCalls) != 1 {
		t.Fatalf("expected create path to persist once, got %d save calls", len(store.saveCalls))
	}
	if len(store.createCalls) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(store.createCalls))
	}
	saved := server.clients[resp.ClientID]
	if saved.ClientSecret != "" {
		t.Fatalf("expected public client secret to be empty")
	}
	if saved.TokenEndpointAuthMode != "none" {
		t.Fatalf("unexpected auth mode: %s", saved.TokenEndpointAuthMode)
	}
	if got, want := saved.Scopes, []string{"openid", "profile", "offline_access"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("unexpected scopes: %#v", got)
	}
	if _, ok := server.clients[resp.ClientID]; !ok {
		t.Fatalf("expected client to be cached in server")
	}
}

func TestRegisterDynamicClientRequiresRedirectURIs(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})

	if _, err := server.RegisterDynamicClient(DynamicClientRequest{
		TokenEndpointAuthMode: "none",
		ClientName:            "demo",
	}); err == nil {
		t.Fatalf("expected redirect_uris to remain required for dynamic clients")
	} else if !strings.Contains(err.Error(), "redirect_uris is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterDynamicClientAllowsEmptyRedirectURIsWhenAllowAnyEnabled(t *testing.T) {
	store := &fakeDynamicClientStore{}
	server := newTestServer(store)

	resp, err := server.RegisterDynamicClient(DynamicClientRequest{
		AllowAnyRedirectURI:   true,
		TokenEndpointAuthMode: "none",
		ClientName:            "demo",
	})
	if err != nil {
		t.Fatalf("expected dynamic client allow_any_redirect_uri to allow empty redirect_uris, got %v", err)
	}
	if resp == nil || resp.ClientID == "" {
		t.Fatalf("expected registered dynamic client response")
	}
	if !resp.AllowAnyRedirectURI {
		t.Fatalf("expected response to preserve allow_any_redirect_uri")
	}
	if len(store.createCalls) != 1 || !store.createCalls[0].AllowAnyRedirectURI {
		t.Fatalf("expected create request to persist allow_any_redirect_uri, got %#v", store.createCalls)
	}
	if !server.clients[resp.ClientID].AllowAnyRedirectURI {
		t.Fatalf("expected server cache to preserve allow_any_redirect_uri")
	}
}

func TestUpdateAndDeleteDynamicClient(t *testing.T) {
	store := &fakeDynamicClientStore{}
	server := newTestServer(store)
	server.clients["oidc-demo"] = Client{
		ClientID:              "oidc-demo",
		ClientSecret:          "secret",
		RedirectURIs:          []string{"https://old.example/callback"},
		Scopes:                []string{"openid"},
		TokenEndpointAuthMode: "client_secret_basic",
		IsDynamic:             true,
		CreatedAt:             time.Unix(100, 0),
	}

	resp, err := server.UpdateDynamicClient("oidc-demo", DynamicClientRequest{
		RedirectURIs:        []string{"https://new.example/callback"},
		AllowAnyRedirectURI: true,
		Scope:               "openid profile offline_access invalid",
		ClientName:          "renamed",
	})
	if err != nil {
		t.Fatalf("UpdateDynamicClient returned error: %v", err)
	}
	if len(store.saveCalls) != 1 || !store.saveCalls[0].isUpdate {
		t.Fatalf("expected one update save call, got %#v", store.saveCalls)
	}
	updated := store.saveCalls[0].client
	if updated.Name != "renamed" {
		t.Fatalf("expected updated name, got %s", updated.Name)
	}
	if len(updated.Scopes) != 3 {
		t.Fatalf("expected filtered scopes, got %#v", updated.Scopes)
	}
	if !updated.AllowAnyRedirectURI {
		t.Fatalf("expected updated client to preserve allow_any_redirect_uri")
	}
	if resp.ClientName != "renamed" {
		t.Fatalf("expected response with updated client name")
	}
	if !resp.AllowAnyRedirectURI {
		t.Fatalf("expected response to include allow_any_redirect_uri")
	}

	if err := server.DeleteDynamicClient("oidc-demo"); err != nil {
		t.Fatalf("DeleteDynamicClient returned error: %v", err)
	}
	if len(store.deleteCalls) != 1 || store.deleteCalls[0] != "oidc-demo" {
		t.Fatalf("unexpected delete calls: %#v", store.deleteCalls)
	}
	if _, ok := server.clients["oidc-demo"]; ok {
		t.Fatalf("expected client removed from cache")
	}
}

func TestNormalizeClientIDProducesRFC1123SafeValue(t *testing.T) {
	tests := map[string]string{
		"oidc_lepdtcdfvtnblji-": "oidc-lepdtcdfvtnblji",
		"-OIDC__Demo--":         "oidc-demo",
		"...":                   "oidc",
	}

	for input, want := range tests {
		if got := normalizeClientID(input); got != want {
			t.Fatalf("normalizeClientID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFindClientFallsBackToStore(t *testing.T) {
	store := &fakeDynamicClientStore{
		loadClients: []Client{
			{
				ClientID:  "oidc-fallback",
				IsDynamic: true,
			},
		},
	}
	server := newTestServer(store)

	client, ok := server.findClient("oidc-fallback")
	if !ok {
		t.Fatalf("expected fallback store lookup to succeed")
	}
	if client.ClientID != "oidc-fallback" {
		t.Fatalf("unexpected client id %q", client.ClientID)
	}
	if len(store.getCalls) != 1 || store.getCalls[0] != "oidc-fallback" {
		t.Fatalf("expected store.Get to be called once, got %#v", store.getCalls)
	}
}

func TestCreateAuthRequestLifecycle(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	req, err := server.CreateAuthRequest(context.Background(), &zitadeloidc.AuthRequest{
		ClientID:            "client-1",
		RedirectURI:         "https://client.example/callback",
		State:               "state-1",
		Scopes:              []string{"openid", "profile"},
		ResponseType:        zitadeloidc.ResponseTypeCode,
		ResponseMode:        zitadeloidc.ResponseModeQuery,
		Nonce:               "nonce-1",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: zitadeloidc.CodeChallengeMethodS256,
	}, "")
	if err != nil {
		t.Fatalf("CreateAuthRequest returned error: %v", err)
	}
	authReq := req.(*authRequest)
	if authReq.Done() {
		t.Fatalf("expected auth request to be pending")
	}
	if authReq.GetCodeChallenge() == nil || authReq.GetCodeChallenge().Challenge != "challenge" {
		t.Fatalf("expected code challenge to be stored")
	}

	if err := server.CompleteAuthRequest(authReq.ID, "alice"); err != nil {
		t.Fatalf("CompleteAuthRequest returned error: %v", err)
	}
	stored, err := server.AuthRequestByID(context.Background(), authReq.ID)
	if err != nil {
		t.Fatalf("AuthRequestByID returned error: %v", err)
	}
	completed := stored.(*authRequest)
	if !completed.Done() || completed.GetSubject() != "alice" {
		t.Fatalf("expected completed auth request for alice, got done=%v subject=%s", completed.Done(), completed.GetSubject())
	}

	if err := server.SaveAuthCode(context.Background(), authReq.ID, "code-1"); err != nil {
		t.Fatalf("SaveAuthCode returned error: %v", err)
	}
	byCode, err := server.AuthRequestByCode(context.Background(), "code-1")
	if err != nil {
		t.Fatalf("AuthRequestByCode returned error: %v", err)
	}
	if byCode.(*authRequest).ID != authReq.ID {
		t.Fatalf("expected auth request lookup by code to return same request")
	}

	if err := server.DeleteAuthRequest(context.Background(), authReq.ID); err != nil {
		t.Fatalf("DeleteAuthRequest returned error: %v", err)
	}
	if _, err := server.AuthRequestByCode(context.Background(), "code-1"); err == nil {
		t.Fatalf("expected deleted auth code lookup to fail")
	}
}

func TestLoginCompletesAuthRequestAfterPasswordValidation(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	req, err := server.CreateAuthRequest(context.Background(), &zitadeloidc.AuthRequest{
		ClientID:     "client-1",
		RedirectURI:  "https://client.example/callback",
		ResponseType: zitadeloidc.ResponseTypeCode,
	}, "")
	if err != nil {
		t.Fatalf("CreateAuthRequest returned error: %v", err)
	}

	if err := server.Login(context.Background(), req.GetID(), "alice", "secret"); err != nil {
		t.Fatalf("Login returned error: %v", err)
	}

	stored, err := server.AuthRequestByID(context.Background(), req.GetID())
	if err != nil {
		t.Fatalf("AuthRequestByID returned error: %v", err)
	}
	if !stored.Done() || stored.GetSubject() != "alice" {
		t.Fatalf("expected login to complete auth request for alice, got done=%v subject=%q", stored.Done(), stored.GetSubject())
	}
	if stored.GetAuthTime().IsZero() {
		t.Fatalf("expected auth time to be set")
	}
}

func TestLoginURLUsesAuthorizeLoginPath(t *testing.T) {
	client := oidcClient{client: Client{ClientID: "client-1"}}
	if got, want := client.LoginURL("request-1"), "/panel-api/v1/oidc/authorize/login?authRequestID=request-1"; got != want {
		t.Fatalf("unexpected login url: got %q want %q", got, want)
	}
}

func TestBuildAuthorizationCallbackURLReturnsFinalRedirectWithCode(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-1"] = Client{
		ClientID:              "client-1",
		RedirectURIs:          []string{"https://client.example/callback"},
		Scopes:                []string{"openid", "profile"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	req, err := server.CreateAuthRequest(context.Background(), &zitadeloidc.AuthRequest{
		ClientID:            "client-1",
		RedirectURI:         "https://client.example/callback",
		Scopes:              []string{"openid", "profile"},
		State:               "state-1",
		ResponseType:        zitadeloidc.ResponseTypeCode,
		ResponseMode:        zitadeloidc.ResponseModeQuery,
		CodeChallenge:       "challenge",
		CodeChallengeMethod: zitadeloidc.CodeChallengeMethodS256,
	}, "alice")
	if err != nil {
		t.Fatalf("CreateAuthRequest returned error: %v", err)
	}

	callbackURL, err := server.BuildAuthorizationCallbackURL(context.Background(), req.GetID())
	if err != nil {
		t.Fatalf("BuildAuthorizationCallbackURL returned error: %v", err)
	}
	if !strings.HasPrefix(callbackURL, "https://client.example/callback?") {
		t.Fatalf("expected final redirect uri, got %q", callbackURL)
	}
	if !strings.Contains(callbackURL, "code=") {
		t.Fatalf("expected callback URL to contain code, got %q", callbackURL)
	}
	if !strings.Contains(callbackURL, "state=state-1") {
		t.Fatalf("expected callback URL to contain state, got %q", callbackURL)
	}
}

func TestCreateDirectAuthorizationCode(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-1"] = Client{
		ClientID:              "client-1",
		RedirectURIs:          []string{"https://client.example/callback"},
		Scopes:                []string{"openid", "profile"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	resp, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		Username:            "alice",
		ClientID:            "client-1",
		RedirectURI:         "https://client.example/callback",
		Scope:               "openid profile",
		State:               "state-1",
		ResponseType:        "code",
		CodeChallenge:       "challenge",
		CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("CreateDirectAuthorizationCode returned error: %v", err)
	}
	if resp.Code == "" {
		t.Fatalf("expected code in response")
	}
	if resp.State != "state-1" {
		t.Fatalf("expected state to round trip, got %q", resp.State)
	}
	byCode, err := server.AuthRequestByCode(context.Background(), resp.Code)
	if err != nil {
		t.Fatalf("AuthRequestByCode returned error: %v", err)
	}
	if byCode.GetSubject() != "alice" {
		t.Fatalf("expected subject alice, got %q", byCode.GetSubject())
	}
}

func TestCreateDirectAuthorizationCodeRejectsInvalidRedirect(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-1"] = Client{
		ClientID:              "client-1",
		RedirectURIs:          []string{"https://client.example/callback"},
		Scopes:                []string{"openid"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	if _, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		ClientID:    "client-1",
		RedirectURI: "https://evil.example/callback",
		Scope:       "openid",
	}); err == nil {
		t.Fatalf("expected invalid redirect_uri to fail")
	}
}

func TestCreateDirectAuthorizationCodeAllowsAnyRedirectWhenInsecureFlagEnabled(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	server.config.InsecureAllowAnyRedirectURI = true
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-1"] = Client{
		ClientID:              "client-1",
		RedirectURIs:          []string{"https://client.example/callback"},
		Scopes:                []string{"openid"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	resp, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		ClientID:    "client-1",
		RedirectURI: "https://evil.example/callback",
		Scope:       "openid",
	})
	if err != nil {
		t.Fatalf("expected insecure redirect override to allow request, got %v", err)
	}
	if resp == nil || resp.Code == "" {
		t.Fatalf("expected authorization code response")
	}
}

func TestCreateDirectAuthorizationCodeAllowsAnyRedirectForStaticClientConfig(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-static-any"] = Client{
		ClientID:              "client-static-any",
		AllowAnyRedirectURI:   true,
		Scopes:                []string{"openid"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	resp, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		ClientID:    "client-static-any",
		RedirectURI: "https://any.example/callback",
		Scope:       "openid",
	})
	if err != nil {
		t.Fatalf("expected static client allow_any_redirect_uri to allow request, got %v", err)
	}
	if resp == nil || resp.Code == "" {
		t.Fatalf("expected authorization code response")
	}
}

func TestCreateDirectAuthorizationCodeAllowsAnyRedirectForStaticClientConfigEvenWithRedirectList(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-static-any-with-list"] = Client{
		ClientID:              "client-static-any-with-list",
		RedirectURIs:          []string{"https://configured.example/callback"},
		AllowAnyRedirectURI:   true,
		Scopes:                []string{"openid"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	resp, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		ClientID:    "client-static-any-with-list",
		RedirectURI: "https://unlisted.example/callback",
		Scope:       "openid",
	})
	if err != nil {
		t.Fatalf("expected allow_any_redirect_uri to override configured redirect list, got %v", err)
	}
	if resp == nil || resp.Code == "" {
		t.Fatalf("expected authorization code response")
	}
}

func TestCodeExchangeRejectsMismatchedRedirectURI(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	server.clients["client-1"] = Client{
		ClientID:              "client-1",
		RedirectURIs:          []string{"https://client.example/callback"},
		Scopes:                []string{"openid", "offline_access"},
		TokenEndpointAuthMode: "none",
		CreatedAt:             time.Now(),
	}
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	resp, err := server.CreateDirectAuthorizationCode(context.Background(), DirectAuthorizeRequest{
		Username:     "alice",
		ClientID:     "client-1",
		RedirectURI:  "https://client.example/callback",
		Scope:        "openid offline_access",
		State:        "state-1",
		ResponseType: "code",
	})
	if err != nil {
		t.Fatalf("CreateDirectAuthorizationCode returned error: %v", err)
	}

	authReq, err := server.AuthRequestByCode(context.Background(), resp.Code)
	if err != nil {
		t.Fatalf("AuthRequestByCode returned error: %v", err)
	}
	if authReq.GetRedirectURI() != "https://client.example/callback" {
		t.Fatalf("unexpected stored redirect uri: %s", authReq.GetRedirectURI())
	}

	requestURL, err := url.Parse("https://panel.example.com/panel-api/v1/oidc/token")
	if err != nil {
		t.Fatalf("url.Parse returned error: %v", err)
	}
	clientReq := &op.ClientRequest[zitadeloidc.AccessTokenRequest]{
		Request: &op.Request[zitadeloidc.AccessTokenRequest]{
			Method: http.MethodPost,
			URL:    requestURL,
			Header: http.Header{"Content-Type": []string{"application/x-www-form-urlencoded"}},
			Form: url.Values{
				"grant_type":   []string{"authorization_code"},
				"client_id":    []string{"client-1"},
				"code":         []string{resp.Code},
				"redirect_uri": []string{"https://evil.example/callback"},
			},
			PostForm: url.Values{
				"grant_type":   []string{"authorization_code"},
				"client_id":    []string{"client-1"},
				"code":         []string{resp.Code},
				"redirect_uri": []string{"https://evil.example/callback"},
			},
			Data: &zitadeloidc.AccessTokenRequest{
				Code:        resp.Code,
				RedirectURI: "https://evil.example/callback",
				ClientID:    "client-1",
			},
		},
		Client: server.newOIDCClient(server.clients["client-1"]),
	}

	if _, err := server.legacy.CodeExchange(context.Background(), clientReq); err == nil {
		t.Fatalf("expected mismatched redirect_uri to fail code exchange")
	} else if !strings.Contains(err.Error(), "redirect_uri does not correspond") {
		t.Fatalf("expected redirect mismatch error, got %v", err)
	}
}

func TestCreateAuthRequestPromptNoneRequiresLogin(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	_, err := server.CreateAuthRequest(context.Background(), &zitadeloidc.AuthRequest{
		ClientID:     "client-1",
		RedirectURI:  "https://client.example/callback",
		Prompt:       []string{zitadeloidc.PromptNone},
		ResponseType: zitadeloidc.ResponseTypeCode,
	}, "")
	if err == nil {
		t.Fatalf("expected prompt=none without user to fail")
	}
}

func TestDiscoveryIssuerDoesNotIncludePath(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	server.private = privateKey
	server.kid = "test-kid"
	if err := server.initProvider(); err != nil {
		t.Fatalf("initProvider returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://panel.example.com/panel-api/v1/oidc/.well-known/openid-configuration", nil)
	resp, err := server.Discovery(context.Background(), req)
	if err != nil {
		t.Fatalf("Discovery returned error: %v", err)
	}

	config, ok := resp.Data.(*zitadeloidc.DiscoveryConfiguration)
	if !ok {
		t.Fatalf("unexpected discovery response type: %T", resp.Data)
	}
	if got, want := config.Issuer, "http://panel.example.com"; got != want {
		t.Fatalf("unexpected issuer: got %s want %s", got, want)
	}
	if got, want := config.AuthorizationEndpoint, "http://panel.example.com/panel-api/v1/oidc/authorize"; got != want {
		t.Fatalf("unexpected authorization endpoint: got %s want %s", got, want)
	}
}

func TestRefreshTokenRotationAndRevocation(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	req := testTokenRequest{
		subject:  "alice",
		audience: []string{"client-1"},
		scopes:   []string{"openid", "offline_access"},
		clientID: "client-1",
		authTime: time.Now().Add(-time.Minute),
		amr:      []string{"pwd"},
	}

	accessTokenID, refreshTokenValue, _, err := server.CreateAccessAndRefreshTokens(context.Background(), req, "")
	if err != nil {
		t.Fatalf("CreateAccessAndRefreshTokens returned error: %v", err)
	}
	refreshReq, err := server.TokenRequestByRefreshToken(context.Background(), refreshTokenValue)
	if err != nil {
		t.Fatalf("TokenRequestByRefreshToken returned error: %v", err)
	}
	if refreshReq.GetSubject() != "alice" {
		t.Fatalf("unexpected refresh token subject: %s", refreshReq.GetSubject())
	}

	if subject, token, err := server.GetRefreshTokenInfo(context.Background(), "client-1", refreshTokenValue); err != nil || subject != "alice" || token != refreshTokenValue {
		t.Fatalf("unexpected refresh token info: subject=%s token=%s err=%v", subject, token, err)
	}

	newAccessTokenID, newRefreshTokenValue, _, err := server.CreateAccessAndRefreshTokens(context.Background(), req, refreshTokenValue)
	if err != nil {
		t.Fatalf("refresh rotation returned error: %v", err)
	}
	if newRefreshTokenValue == refreshTokenValue {
		t.Fatalf("expected rotated refresh token")
	}
	if _, ok := server.refreshTokens[refreshTokenValue]; ok {
		t.Fatalf("expected old refresh token removed")
	}
	if _, ok := server.accessTokens[accessTokenID]; ok {
		t.Fatalf("expected old access token removed after rotation")
	}
	if oidcErr := server.RevokeToken(context.Background(), newRefreshTokenValue, "", "other-client"); oidcErr == nil {
		t.Fatalf("expected revoke with wrong client to fail")
	}
	if oidcErr := server.RevokeToken(context.Background(), newRefreshTokenValue, "", "client-1"); oidcErr != nil {
		t.Fatalf("expected revoke to succeed, got %v", oidcErr)
	}
	if _, ok := server.refreshTokens[newRefreshTokenValue]; ok {
		t.Fatalf("expected refresh token removed after revoke")
	}
	if _, ok := server.accessTokens[newAccessTokenID]; ok {
		t.Fatalf("expected access token removed after revoke")
	}
}

func TestTerminateSessionRemovesClientTokens(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	server.accessTokens["at-1"] = &accessToken{ID: "at-1", ApplicationID: "client-1", Subject: "alice", Expiration: time.Now().Add(time.Hour)}
	server.accessTokens["at-2"] = &accessToken{ID: "at-2", ApplicationID: "client-2", Subject: "alice", Expiration: time.Now().Add(time.Hour)}
	server.refreshTokens["rt-1"] = &refreshToken{Token: "rt-1", ApplicationID: "client-1", Subject: "alice", AccessTokenID: "at-1", Expiration: time.Now().Add(time.Hour)}
	server.refreshTokens["rt-2"] = &refreshToken{Token: "rt-2", ApplicationID: "client-2", Subject: "alice", AccessTokenID: "at-2", Expiration: time.Now().Add(time.Hour)}

	if err := server.TerminateSession(context.Background(), "alice", "client-1"); err != nil {
		t.Fatalf("TerminateSession returned error: %v", err)
	}
	if _, ok := server.accessTokens["at-1"]; ok {
		t.Fatalf("expected client-1 access token removed")
	}
	if _, ok := server.refreshTokens["rt-1"]; ok {
		t.Fatalf("expected client-1 refresh token removed")
	}
	if _, ok := server.accessTokens["at-2"]; !ok {
		t.Fatalf("expected other client access token to remain")
	}
}

func TestSigningKeyPEMRoundTrip(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey returned error: %v", err)
	}
	pemValue := normalizePrivateKeyPEM(privateKey)

	parsed, err := parsePrivateKeyPEM(pemValue)
	if err != nil {
		t.Fatalf("parsePrivateKeyPEM returned error: %v", err)
	}
	if parsed.D.Cmp(privateKey.D) != 0 {
		t.Fatalf("expected parsed key to match original")
	}

	fromFallback, normalized, err := buildInitialSigningKey(pemValue)
	if err != nil {
		t.Fatalf("buildInitialSigningKey returned error: %v", err)
	}
	if fromFallback.D.Cmp(privateKey.D) != 0 {
		t.Fatalf("expected fallback key to match original")
	}
	if normalized == "" {
		t.Fatalf("expected normalized pem")
	}
}

func TestSigningKeyPEMRejectsInvalidInput(t *testing.T) {
	if _, err := parsePrivateKeyPEM("not-a-pem"); err == nil {
		t.Fatalf("expected invalid pem to fail")
	}
	if _, err := parseSigningKeySecret(nil); err == nil {
		t.Fatalf("expected nil secret to fail")
	}
}
