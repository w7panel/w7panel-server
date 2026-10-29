package oidc

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type mcpResourceKey struct{}

func (s *Server) TokenEndpointAuthMethods() []string {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	methods := make([]string, 0, 3)
	for _, method := range []string{"none", "client_secret_basic", "client_secret_post"} {
		for _, client := range s.clients {
			if client.TokenEndpointAuthMode == method {
				methods = append(methods, method)
				break
			}
		}
	}
	return methods
}

func mcpResourceFromContext(ctx context.Context) string {
	resource, _ := ctx.Value(mcpResourceKey{}).(string)
	return resource
}

func (s *Server) MCPResource(r *http.Request) string {
	return s.discoveryIssuer(r) + "/panel-api/v1/mcp"
}

// WithMCPResource binds authorization codes to the MCP resource requested by the client.
func (s *Server) WithMCPResource(r *http.Request) (*http.Request, error) {
	resources := r.URL.Query()["resource"]
	if len(resources) == 0 {
		return r, nil
	}
	if len(resources) != 1 || resources[0] != s.MCPResource(r) {
		return nil, errors.New("invalid resource")
	}
	query := r.URL.Query()
	if !containsString(strings.Fields(query.Get("scope")), "mcp") || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		return nil, errors.New("MCP authorization requires mcp scope and S256 PKCE")
	}
	return r.WithContext(context.WithValue(r.Context(), mcpResourceKey{}, resources[0])), nil
}

// VerifyMCPAccessToken accepts only active OIDC access tokens issued for this MCP endpoint.
func (s *Server) VerifyMCPAccessToken(r *http.Request, raw string) (string, error) {
	if !s.Enabled() {
		return "", errors.New("oidc disabled")
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(s.Issuer(r)),
		jwt.WithAudience(s.MCPResource(r)),
		jwt.WithExpirationRequired(),
	).ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		return &s.private.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid oidc access token")
	}
	id, _ := claims["jti"].(string)
	subject, _ := claims["sub"].(string)
	clientID, _ := claims["client_id"].(string)
	if id == "" || subject == "" || clientID == "" {
		return "", errors.New("incomplete oidc access token")
	}
	s.tokenMu.Lock()
	stored := s.accessTokens[id]
	s.tokenMu.Unlock()
	if stored == nil || stored.Subject != subject || stored.ApplicationID != clientID || !time.Now().Before(stored.Expiration) ||
		!containsString(stored.Audience, s.MCPResource(r)) || !containsString(stored.Scopes, "mcp") {
		return "", errors.New("inactive oidc access token")
	}
	return subject, nil
}
