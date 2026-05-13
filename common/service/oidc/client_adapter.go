package oidc

import (
	"time"

	zitadeloidc "github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type oidcClient struct {
	client              Client
	allowAnyRedirectURI bool
}

func (c oidcClient) GetID() string { return c.client.ClientID }
func (c oidcClient) RedirectURIs() []string {
	return c.client.RedirectURIs
}
func (c oidcClient) RedirectURIGlobs() []string {
	if c.allowAnyRedirectURI {
		return []string{"**"}
	}
	return nil
}
func (c oidcClient) PostLogoutRedirectURIs() []string { return nil }
func (c oidcClient) PostLogoutRedirectURIGlobs() []string {
	if c.allowAnyRedirectURI {
		return []string{"**"}
	}
	return nil
}
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
	return []zitadeloidc.ResponseType{zitadeloidc.ResponseTypeCode, zitadeloidc.ResponseTypeIDToken, zitadeloidc.ResponseTypeIDTokenOnly}
}
func (c oidcClient) GrantTypes() []zitadeloidc.GrantType {
	grants := []zitadeloidc.GrantType{zitadeloidc.GrantTypeCode}
	if containsString(c.client.Scopes, zitadeloidc.ScopeOfflineAccess) {
		grants = append(grants, zitadeloidc.GrantTypeRefreshToken)
	}
	return grants
}
func (c oidcClient) LoginURL(id string) string {
	return "/login?" + authRequestIDQuery + "=" + id
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
