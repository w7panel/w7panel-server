package oidc

import (
	"context"
	"errors"
	"strings"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type cachedAccessToken struct {
	Token      string
	Expiration time.Time
}

func (s *Server) getOrCreateDefaultAccessToken(ctx context.Context, subject string) (string, error) {
	if strings.TrimSpace(subject) == "" {
		return "", errors.New("subject is required")
	}

	now := time.Now()

	s.accessTokenMu.Lock()
	cached := s.accessTokenBy[subject]
	if cached != nil && cached.Token != "" && now.Before(cached.Expiration) {
		token := cached.Token
		s.accessTokenMu.Unlock()
		return token, nil
	}
	s.accessTokenMu.Unlock()

	token, exp, err := s.createDefaultAccessToken(ctx, subject)
	if err != nil {
		return "", err
	}

	s.accessTokenMu.Lock()
	s.accessTokenBy[subject] = &cachedAccessToken{
		Token:      token,
		Expiration: exp,
	}
	s.accessTokenMu.Unlock()

	return token, nil
}

func (s *Server) createDefaultAccessToken(ctx context.Context, subject string) (string, time.Time, error) {
	clientConfig, ok := s.findClient("default")
	if !ok {
		return "", time.Time{}, errors.New("client not found")
	}
	client, err := s.GetClientByClientID(ctx, "default")
	if err != nil {
		return "", time.Time{}, err
	}

	issuer := op.IssuerFromContext(ctx)
	if issuer == "" {
		issuer = strings.TrimRight(s.config.Issuer, "/")
	}
	if issuer == "" {
		return "", time.Time{}, errors.New("issuer is required")
	}
	ctx = op.ContextWithIssuer(ctx, issuer)

	scopes := append([]string{}, clientConfig.Scopes...)
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile"}
	}

	req := defaultAccessTokenRequest{
		subject:  subject,
		audience: []string{client.GetID()},
		scopes:   scopes,
		clientID: client.GetID(),
	}
	accessToken, _, _, err := op.CreateAccessToken(ctx, req, client.AccessTokenType(), s.provider, client, "")
	if err != nil {
		return "", time.Time{}, err
	}

	exp, err := accessTokenExpiration(accessToken)
	if err != nil {
		exp = time.Now().Add(s.config.AccessTokenTTL)
	}
	return accessToken, exp, nil
}

func accessTokenExpiration(token string) (time.Time, error) {
	claims := jwtv5.MapClaims{}
	if _, _, err := jwtv5.NewParser().ParseUnverified(token, claims); err != nil {
		return time.Time{}, err
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		if err == nil {
			err = errors.New("exp claim missing")
		}
		return time.Time{}, err
	}
	return exp.Time, nil
}
