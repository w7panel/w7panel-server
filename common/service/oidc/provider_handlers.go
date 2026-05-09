package oidc

import (
	"context"
	"errors"
	"time"

	"github.com/go-jose/go-jose/v4"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

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
