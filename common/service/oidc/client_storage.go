package oidc

import (
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type dynamicClientStore interface {
	Load() ([]Client, error)
	Save(client Client, isUpdate bool) error
	Delete(clientID string) error
}

type kubeDynamicClientStore struct{}

func newDynamicClientStore() dynamicClientStore {
	return kubeDynamicClientStore{}
}

func (kubeDynamicClientStore) Load() ([]Client, error) {
	sdk := k8s.NewK8sClient().Sdk
	secrets, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).List(sdk.Ctx, metav1.ListOptions{
		LabelSelector: "w7.cc/oidc-client=true",
	})
	if err != nil {
		return nil, err
	}
	clients := make([]Client, 0, len(secrets.Items))
	for _, secret := range secrets.Items {
		client := clientFromSecret(&secret)
		if client.ClientID != "" {
			clients = append(clients, client)
		}
	}
	return clients, nil
}

func (kubeDynamicClientStore) Save(client Client, isUpdate bool) error {
	sdk := k8s.NewK8sClient().Sdk
	secret := secretFromClient(sdk.GetNamespace(), client)
	if isUpdate {
		current, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, client.ClientID, metav1.GetOptions{})
		if err != nil {
			return err
		}
		secret.ResourceVersion = current.ResourceVersion
		_, err = sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Update(sdk.Ctx, secret, metav1.UpdateOptions{})
		return err
	}
	_, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Create(sdk.Ctx, secret, metav1.CreateOptions{})
	return err
}

func (kubeDynamicClientStore) Delete(clientID string) error {
	sdk := k8s.NewK8sClient().Sdk
	err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Delete(sdk.Ctx, clientID, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func clientFromSecret(secret *corev1.Secret) Client {
	return Client{
		Name:                  string(secret.Data["client_name"]),
		ClientID:              secret.Name,
		ClientSecret:          string(secret.Data["client_secret"]),
		RedirectURIs:          splitLines(string(secret.Data["redirect_uris"])),
		Scopes:                normalizeScopes(strings.Fields(string(secret.Data["scopes"]))),
		RequirePKCE:           string(secret.Data["require_pkce"]) == "true",
		TokenEndpointAuthMode: string(secret.Data["token_endpoint_auth_method"]),
		IsDynamic:             secret.Labels["w7.cc/oidc-client"] == "true",
		CreatedAt:             secret.CreationTimestamp.Time,
	}
}

func secretFromClient(namespace string, client Client) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      client.ClientID,
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/oidc-client": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client_name":                []byte(client.Name),
			"client_secret":              []byte(client.ClientSecret),
			"redirect_uris":              []byte(strings.Join(client.RedirectURIs, "\n")),
			"scopes":                     []byte(strings.Join(client.Scopes, " ")),
			"require_pkce":               []byte(fmt.Sprintf("%t", client.RequirePKCE)),
			"token_endpoint_auth_method": []byte(client.TokenEndpointAuthMode),
		},
	}
}

func clientToResponse(client Client) *DynamicClientResponse {
	return &DynamicClientResponse{
		ClientID:              client.ClientID,
		ClientSecret:          client.ClientSecret,
		ClientIDIssuedAt:      client.CreatedAt.Unix(),
		ClientSecretExpiresAt: 0,
		RedirectURIs:          client.RedirectURIs,
		TokenEndpointAuthMode: client.TokenEndpointAuthMode,
		GrantTypes:            []string{"authorization_code", "refresh_token"},
		ResponseTypes:         []string{"code"},
		Scope:                 strings.Join(client.Scopes, " "),
		ClientName:            client.Name,
		RequirePKCE:           client.RequirePKCE,
		RegistrationClientURI: "/panel-api/v1/oidc/register/" + client.ClientID,
	}
}
