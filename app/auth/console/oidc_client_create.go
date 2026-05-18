package console

import (
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/oidc"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
)

type OidcClientCreate struct {
	console2.Abstract
}

type oidcClientCreateOption struct {
	ClientName            string
	RedirectURIs          []string
	AllowAnyRedirectURI   bool
	TokenEndpointAuthMode string
	GrantTypes            []string
	Scope                 string
}

var oidcCreateOption = oidcClientCreateOption{}

func (c OidcClientCreate) GetName() string {
	return "oidc:client-create"
}

// go run main.go oidc:client-create --client-name=gitea --allow-any-redirect-uri=true --token-endpoint-auth-method=client_secret_post --scope="openid offline_access profile"
func (c OidcClientCreate) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&oidcCreateOption.ClientName, "client-name", "", "client name")
	cmd.Flags().StringArrayVar(&oidcCreateOption.RedirectURIs, "redirect-uri", nil, "redirect uri, repeatable")
	cmd.Flags().BoolVar(&oidcCreateOption.AllowAnyRedirectURI, "allow-any-redirect-uri", false, "allow any redirect uri")
	cmd.Flags().StringVar(&oidcCreateOption.TokenEndpointAuthMode, "token-endpoint-auth-method", "", "client_secret_basic/client_secret_post/none")
	cmd.Flags().StringArrayVar(&oidcCreateOption.GrantTypes, "grant-type", nil, "grant type, repeatable")
	cmd.Flags().StringVar(&oidcCreateOption.Scope, "scope", "", "scope list separated by spaces")
}

func (c OidcClientCreate) GetDescription() string {
	return "创建 OIDC dynamic client"
}

// go run main.go oidc:client-create --client-name=gitea --allow-any-redirect-uri=true --token-endpoint-auth-method=client_secret_post --scope=openid offline_access profile
func (c OidcClientCreate) Handle(cmd *cobra.Command, args []string) {
	resp, err := oidc.CreateDynamicClient(oidc.DynamicClientRequest{
		RedirectURIs:          oidcCreateOption.RedirectURIs,
		AllowAnyRedirectURI:   oidcCreateOption.AllowAnyRedirectURI,
		TokenEndpointAuthMode: oidcCreateOption.TokenEndpointAuthMode,
		GrantTypes:            oidcCreateOption.GrantTypes,
		Scope:                 oidcCreateOption.Scope,
		ClientName:            oidcCreateOption.ClientName,
	})
	if err != nil {
		slog.Error("create oidc client failed", "err", err)
		return
	}

	slog.Info("oidc client created",
		"client_id", resp.ClientID,
		"client_secret", resp.ClientSecret,
		"client_name", resp.ClientName,
		"redirect_uris", resp.RedirectURIs,
		"allow_any_redirect_uri", resp.AllowAnyRedirectURI,
		"token_endpoint_auth_method", resp.TokenEndpointAuthMode,
		"scope", resp.Scope,
	)
}
