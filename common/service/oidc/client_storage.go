package oidc

import (
	"strconv"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type LoadSecretFunc func(name string) (*corev1.Secret, error)

var loadFunc LoadSecretFunc

func SetLoadFunc(f LoadSecretFunc) {
	loadFunc = f
}

type dynamicClientStore interface {
	Load() ([]Client, error)
	Get(clientID string) (Client, error)
	Create(req DynamicClientRequest) (Client, error)
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

func (kubeDynamicClientStore) Get(clientID string) (Client, error) {
	sdk := k8s.NewK8sClient().Sdk
	secret, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, secretNameForClientID(clientID), metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			if loadFunc != nil {
				secret, err := loadFunc(clientID)

				if err == nil {
					return clientFromSecret(secret), nil
				}
			}
		}
		return Client{}, err
	}
	return clientFromSecret(secret), nil
}

func (kubeDynamicClientStore) Create(req DynamicClientRequest) (Client, error) {
	mode := normalizeAuthMethod(req.TokenEndpointAuthMode, "x")
	client := Client{
		Name:                  req.ClientName,
		ClientID:              normalizeClientID("oidc_" + randomToken(16)),
		RedirectURIs:          req.RedirectURIs,
		AllowAnyRedirectURI:   req.AllowAnyRedirectURI,
		Scopes:                normalizeScopes(strings.Fields(req.Scope)),
		TokenEndpointAuthMode: mode,
		IsDynamic:             true,
		CreatedAt:             time.Now(),
	}
	if len(client.Scopes) == 0 {
		client.Scopes = []string{"openid", "profile", "offline_access"}
	}
	if mode != "none" {
		client.ClientSecret = randomToken(24)
	}
	if err := (kubeDynamicClientStore{}).Save(client, false); err != nil {
		return Client{}, err
	}
	return client, nil
}

func (kubeDynamicClientStore) Save(client Client, isUpdate bool) error {
	sdk := k8s.NewK8sClient().Sdk
	secret := secretFromClient(sdk.GetNamespace(), client)
	if isUpdate {
		current, err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Get(sdk.Ctx, secretNameForClientID(client.ClientID), metav1.GetOptions{})
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
	err := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace()).Delete(sdk.Ctx, secretNameForClientID(clientID), metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func clientFromSecret(secret *corev1.Secret) Client {
	clientID := string(secret.Data["client_id"])
	if clientID == "" {
		clientID = strings.TrimSuffix(secret.Name, "-oidc")
	}
	return Client{
		Name:                  string(secret.Data["client_name"]),
		ClientID:              clientID,
		ClientSecret:          string(secret.Data["client_secret"]),
		RedirectURIs:          splitLines(string(secret.Data["redirect_uris"])),
		AllowAnyRedirectURI:   parseBool(secret.Data["allow_any_redirect_uri"]),
		Scopes:                normalizeScopes(strings.Fields(string(secret.Data["scopes"]))),
		TokenEndpointAuthMode: string(secret.Data["token_endpoint_auth_method"]),
		IsDynamic:             secret.Labels["w7.cc/oidc-client"] == "true",
		CreatedAt:             secret.CreationTimestamp.Time,
	}
}

func secretFromClient(namespace string, client Client) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretNameForClientID(client.ClientID),
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/oidc-client": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client_id":                  []byte(client.ClientID),
			"client_name":                []byte(client.Name),
			"client_secret":              []byte(client.ClientSecret),
			"redirect_uris":              []byte(strings.Join(client.RedirectURIs, "\n")),
			"allow_any_redirect_uri":     []byte(strconv.FormatBool(client.AllowAnyRedirectURI)),
			"scopes":                     []byte(strings.Join(client.Scopes, " ")),
			"token_endpoint_auth_method": []byte(client.TokenEndpointAuthMode),
		},
	}
}

func secretNameForClientID(clientID string) string {
	return clientID + "-oidc"
}

func clientToResponse(client Client) *DynamicClientResponse {
	return &DynamicClientResponse{
		ClientID:              client.ClientID,
		ClientSecret:          client.ClientSecret,
		ClientIDIssuedAt:      client.CreatedAt.Unix(),
		ClientSecretExpiresAt: 0,
		RedirectURIs:          client.RedirectURIs,
		AllowAnyRedirectURI:   client.AllowAnyRedirectURI,
		TokenEndpointAuthMode: client.TokenEndpointAuthMode,
		GrantTypes:            []string{"authorization_code", "refresh_token"},
		Scope:                 strings.Join(client.Scopes, " "),
		ClientName:            client.Name,
	}
}

func parseBool(data []byte) bool {
	v, err := strconv.ParseBool(string(data))
	if err != nil {
		return false
	}
	return v
}
