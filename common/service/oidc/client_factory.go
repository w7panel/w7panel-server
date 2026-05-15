package oidc

func (s *Server) newOIDCClient(client Client) oidcClient {
	return oidcClient{
		client:              client,
		allowAnyRedirectURI: client.AllowAnyRedirectURI || (s != nil && s.config.InsecureAllowAnyRedirectURI),
	}
}
