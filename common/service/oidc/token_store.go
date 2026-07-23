package oidc

import (
	"context"
	"crypto/rsa"
	"errors"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

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

	newRefreshToken := randomToken(32)
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

func (r *refreshTokenRequest) GetAMR() []string                 { return r.token.AMR }
func (r *refreshTokenRequest) GetAudience() []string            { return r.token.Audience }
func (r *refreshTokenRequest) GetAuthTime() time.Time           { return r.token.AuthTime }
func (r *refreshTokenRequest) GetClientID() string              { return r.token.ApplicationID }
func (r *refreshTokenRequest) GetScopes() []string              { return r.token.Scopes }
func (r *refreshTokenRequest) GetSubject() string               { return r.token.Subject }
func (r *refreshTokenRequest) SetCurrentScopes(scopes []string) { r.token.Scopes = scopes }
func (s *signingKey) SignatureAlgorithm() jose.SignatureAlgorithm {
	return s.algorithm
}
func (s *signingKey) Key() any                          { return s.key }
func (s *signingKey) ID() string                        { return s.id }
func (p *publicKey) ID() string                         { return p.id }
func (p *publicKey) Algorithm() jose.SignatureAlgorithm { return p.algorithm }
func (p *publicKey) Use() string                        { return "sig" }
func (p *publicKey) Key() any                           { return &p.key.PublicKey }
