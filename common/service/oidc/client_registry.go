package oidc

import (
	"errors"
	"fmt"
	"strings"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

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
	if len(req.GrantTypes) > 0 && !sameStrings(req.GrantTypes, []string{"authorization_code"}) &&
		!sameStrings(req.GrantTypes, []string{"authorization_code", "refresh_token"}) {
		return nil, errors.New("only authorization_code and refresh_token grant_types are supported")
	}
	mode := normalizeAuthMethod(req.TokenEndpointAuthMode, "x")
	if mode != "client_secret_basic" && mode != "client_secret_post" && mode != "none" {
		return nil, errors.New("unsupported token_endpoint_auth_method")
	}

	client, err := s.store.Create(req)
	if err != nil {
		return nil, err
	}
	s.clientsMu.Lock()
	s.clients[client.ClientID] = client
	s.clientsMu.Unlock()
	return clientToResponse(client), nil
}

func (s *Server) GetDynamicClient(clientID string) (*DynamicClientResponse, error) {
	client, ok := s.findClient(clientID)
	if !ok || !client.IsDynamic {
		return nil, errors.New("client not found")
	}
	return clientToResponse(client), nil
}

func (s *Server) UpdateDynamicClient(clientID string, req DynamicClientRequest) (*DynamicClientResponse, error) {
	client, ok := s.findClient(clientID)
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
	if req.TokenEndpointAuthMode != "" {
		client.TokenEndpointAuthMode = req.TokenEndpointAuthMode
	}
	if err := s.store.Save(client, true); err != nil {
		return nil, err
	}
	s.clientsMu.Lock()
	s.clients[client.ClientID] = client
	s.clientsMu.Unlock()
	return clientToResponse(client), nil
}

func (s *Server) DeleteDynamicClient(clientID string) error {
	client, ok := s.findClient(clientID)
	if !ok || !client.IsDynamic {
		return errors.New("client not found")
	}
	if err := s.store.Delete(client.ClientID); err != nil {
		return err
	}
	s.clientsMu.Lock()
	delete(s.clients, client.ClientID)
	s.clientsMu.Unlock()
	return nil
}

func (s *Server) findClient(clientID string) (Client, bool) {
	s.clientsMu.RLock()
	client, ok := s.clients[clientID]
	s.clientsMu.RUnlock()
	if ok {
		return client, true
	}

	client, err := s.store.Get(clientID)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return Client{}, false
		}
		return Client{}, false
	}
	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()
	return client, true
}
