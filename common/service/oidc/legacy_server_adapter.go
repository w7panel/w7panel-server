package oidc

import (
	"context"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type legacyServerAdapters struct {
	*op.LegacyServer
	provider *op.Provider
}

func newLegacyServerAdapters(legacy *op.LegacyServer, provider *op.Provider) op.ExtendedLegacyServer {
	return &legacyServerAdapters{
		LegacyServer: legacy,
		provider:     provider,
	}
}

func (s *legacyServerAdapters) Provider() op.OpenIDProvider {
	return s.provider
}
func (s *legacyServerAdapters) Endpoints() op.Endpoints {
	return s.LegacyServer.Endpoints()
}

func (s *legacyServerAdapters) AuthCallbackURL() func(context.Context, string) string {
	return s.LegacyServer.AuthCallbackURL()
}

func (s *legacyServerAdapters) CodeExchange(ctx context.Context, r *op.ClientRequest[oidc.AccessTokenRequest]) (*op.Response, error) {

	authReq, err := op.AuthRequestByCode(ctx, s.Provider().Storage(), r.Data.Code)
	if err != nil {
		return nil, err
	}
	if r.Client.AuthMethod() == oidc.AuthMethodNone || r.Data.CodeVerifier != "" {
		if err = op.AuthorizeCodeChallenge(r.Data.CodeVerifier, authReq.GetCodeChallenge()); err != nil {
			return nil, err
		}
	}
	// if r.Data.RedirectURI != authReq.GetRedirectURI() {
	// 	return nil, oidc.ErrInvalidGrant().WithDescription("redirect_uri does not correspond")
	// }
	resp, err := op.CreateTokenResponse(ctx, authReq, r.Client, s.provider, true, r.Data.Code, "")
	if err != nil {
		return nil, err
	}
	return op.NewResponse(resp), nil
}

// func (s *LegacyServer) Discovery(ctx context.Context, r *op.Request[struct{}]) (*op.Response, error) {
// 	return s.LegacyServer.Discovery(ctx, r)
// }
