package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
)

func TestMCPResourceAndAccessToken(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	server.config.Enabled = true
	server.config.Issuer = "https://panel.example/panel-api/v1/oidc"
	var err error
	server.private, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server.clients["default"] = Client{ClientID: "default"}
	baseQuery := url.Values{"resource": {"https://panel.example/panel-api/v1/mcp"}, "client_id": {"default"}, "scope": {"openid mcp"}, "code_challenge_method": {"S256"}, "code_challenge": {"challenge"}, "redirect_uri": {"http://127.0.0.1:32000/callback"}}
	request := httptest.NewRequest("GET", "https://panel.example/panel-api/v1/oidc/authorize?"+baseQuery.Encode(), nil)
	request, err = server.WithMCPResource(request)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := server.CreateAuthRequest(request.Context(), &zitadeloidc.AuthRequest{ClientID: "default"}, "alice")
	if err != nil || len(auth.GetAudience()) != 1 || auth.GetAudience()[0] != server.MCPResource(request) {
		t.Fatalf("MCP audience not preserved: %v, %v", auth, err)
	}
	baseQuery.Set("resource", "https://evil.example")
	wrong := httptest.NewRequest("GET", "https://panel.example/panel-api/v1/oidc/authorize?"+baseQuery.Encode(), nil)
	if _, err := server.WithMCPResource(wrong); err == nil {
		t.Fatal("foreign resource accepted")
	}
	baseQuery.Set("resource", server.MCPResource(request))
	baseQuery.Set("redirect_uri", "https://evil.example/callback")
	wrong = httptest.NewRequest("GET", "https://panel.example/panel-api/v1/oidc/authorize?"+baseQuery.Encode(), nil)
	if _, err := server.WithMCPResource(wrong); err != nil {
		t.Fatalf("existing arbitrary redirect policy changed: %v", err)
	}

	expires := time.Now().Add(time.Hour)
	server.accessTokens["token-1"] = &accessToken{ID: "token-1", ApplicationID: "default", Subject: "alice", Audience: []string{server.MCPResource(request)}, Scopes: []string{"mcp"}, Expiration: expires}
	claims := jwt.MapClaims{
		"iss": server.config.Issuer, "aud": []string{server.MCPResource(request)}, "exp": expires.Unix(),
		"jti": "token-1", "sub": "alice", "client_id": "default",
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(server.private)
	if err != nil {
		t.Fatal(err)
	}
	if subject, err := server.VerifyMCPAccessToken(request, raw); err != nil || subject != "alice" {
		t.Fatalf("valid MCP token rejected: %q, %v", subject, err)
	}
	claims["aud"] = []string{"https://other.example/panel-api/v1/mcp"}
	foreign, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(server.private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.VerifyMCPAccessToken(request, foreign); err == nil {
		t.Fatal("token for another resource accepted")
	}
	claims["aud"] = []string{server.MCPResource(request)}
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	expired, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(server.private)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.VerifyMCPAccessToken(request, expired); err == nil {
		t.Fatal("expired token accepted")
	}
	server.accessTokens["token-1"].Scopes = []string{"openid"}
	if _, err := server.VerifyMCPAccessToken(request, raw); err == nil {
		t.Fatal("token without MCP scope accepted")
	}
	server.accessTokens["token-1"].Scopes = []string{"mcp"}
	delete(server.accessTokens, "token-1")
	if _, err := server.VerifyMCPAccessToken(request, raw); err == nil {
		t.Fatal("revoked MCP token accepted")
	}
}

func TestMCPAuthorizationKeepsResourceAudience(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	server.config.Enabled = true
	server.config.Issuer = "https://panel.example/panel-api/v1/oidc"
	var err error
	server.private, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server.clients["default"] = Client{ClientID: "default", AllowAnyRedirectURI: true, TokenEndpointAuthMode: "none", Scopes: []string{"openid", "mcp", "offline_access"}}
	if err := server.initProvider(); err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("a", 43)
	challengeHash := sha256.Sum256([]byte(verifier))
	callbackURI := "http://localhost/callback"
	resource := "https://panel.example/panel-api/v1/mcp"
	query := url.Values{"response_type": {"code"}, "client_id": {"default"}, "redirect_uri": {callbackURI}, "scope": {"openid mcp offline_access"}, "state": {"abc"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challengeHash[:])}, "code_challenge_method": {"S256"}, "resource": {resource}}
	request := httptest.NewRequest("GET", "https://panel.example/panel-api/v1/oidc/authorize?"+query.Encode(), nil)
	request, err = server.WithMCPResource(request)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != 302 || len(server.authRequests) != 1 {
		t.Fatalf("authorization did not create request: status=%d body=%s requests=%d", response.Code, response.Body.String(), len(server.authRequests))
	}
	for id, auth := range server.authRequests {
		if auth.Resource != server.MCPResource(request) {
			t.Fatalf("resource audience lost: %q", auth.Resource)
		}
		if err := server.CompleteAuthRequest(id, "alice"); err != nil {
			t.Fatal(err)
		}
		callback, err := server.BuildAuthorizationCallbackURL(request.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(callback)
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{"grant_type": {"authorization_code"}, "code": {parsed.Query().Get("code")}, "redirect_uri": {callbackURI}, "client_id": {"default"}, "code_verifier": {verifier}, "resource": {resource}}
		tokenRequest := httptest.NewRequest("POST", "https://panel.example/panel-api/v1/oidc/token", strings.NewReader(form.Encode()))
		tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		tokenResponse := httptest.NewRecorder()
		server.ServeHTTP(tokenResponse, tokenRequest)
		var body struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
		}
		if err := json.Unmarshal(tokenResponse.Body.Bytes(), &body); err != nil || body.AccessToken == "" {
			t.Fatalf("token exchange failed: status=%d body=%s err=%v", tokenResponse.Code, tokenResponse.Body.String(), err)
		}
		idClaims := jwt.MapClaims{}
		idToken, err := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(server.config.Issuer), jwt.WithAudience("default")).ParseWithClaims(body.IDToken, idClaims, func(*jwt.Token) (any, error) {
			return &server.private.PublicKey, nil
		})
		if err != nil || !idToken.Valid || idClaims["azp"] != "default" {
			t.Fatalf("ID token client audience invalid: claims=%v err=%v", idClaims, err)
		}
		if username, err := server.VerifyMCPAccessToken(tokenRequest, body.AccessToken); err != nil || username != "alice" {
			t.Fatalf("exchanged token rejected: username=%q err=%v", username, err)
		}
		if body.RefreshToken == "" {
			t.Fatal("offline_access did not issue refresh token")
		}
		refresh := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {body.RefreshToken}, "client_id": {"default"}, "resource": {resource}}
		refreshRequest := httptest.NewRequest("POST", "https://panel.example/panel-api/v1/oidc/token", strings.NewReader(refresh.Encode()))
		refreshRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		refreshResponse := httptest.NewRecorder()
		server.ServeHTTP(refreshResponse, refreshRequest)
		body.AccessToken = ""
		if err := json.Unmarshal(refreshResponse.Body.Bytes(), &body); err != nil || body.AccessToken == "" {
			t.Fatalf("refresh failed: status=%d body=%s err=%v", refreshResponse.Code, refreshResponse.Body.String(), err)
		}
		if _, err := server.VerifyMCPAccessToken(refreshRequest, body.AccessToken); err != nil {
			t.Fatalf("refreshed MCP token rejected: %v", err)
		}
	}
}

func TestTokenEndpointAuthMethods(t *testing.T) {
	server := newTestServer(&fakeDynamicClientStore{})
	server.clients["default"] = Client{ClientID: "default", TokenEndpointAuthMode: "none"}
	server.clients["basic"] = Client{ClientID: "basic", TokenEndpointAuthMode: "client_secret_basic"}
	server.clients["post"] = Client{ClientID: "post", TokenEndpointAuthMode: "client_secret_post"}
	if got := strings.Join(server.TokenEndpointAuthMethods(), ","); got != "none,client_secret_basic,client_secret_post" {
		t.Fatalf("unexpected token auth methods: %s", got)
	}
	delete(server.clients, "default")
	if got := strings.Join(server.TokenEndpointAuthMethods(), ","); got != "client_secret_basic,client_secret_post" {
		t.Fatalf("disabled public client still advertised: %s", got)
	}
}
