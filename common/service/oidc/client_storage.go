package oidc

import (
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	oidcv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/oidc/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	oidcClients := &oidcv1alpha1.OIDCClientList{}
	if err := sigClient.List(sdk.Ctx, oidcClients, sigclient.InNamespace(sdk.GetNamespace())); err != nil {
		return nil, err
	}
	clients := make([]Client, 0, len(oidcClients.Items))
	for i := range oidcClients.Items {
		client := clientFromOIDCClient(&oidcClients.Items[i])
		if client.ClientID != "" {
			clients = append(clients, client)
		}
	}
	return clients, nil
}

func (kubeDynamicClientStore) Get(clientID string) (Client, error) {
	sdk := k8s.NewK8sClient().Sdk
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return Client{}, err
	}
	item := &oidcv1alpha1.OIDCClient{}
	err = sigClient.Get(sdk.Ctx, sigclient.ObjectKey{Name: resourceNameForClientID(clientID), Namespace: sdk.GetNamespace()}, item)
	if err == nil {
		return clientFromOIDCClient(item), nil
	}
	if k8serrors.IsNotFound(err) && loadFunc != nil {
		secret, loadErr := loadFunc(clientID)
		if loadErr == nil {
			return clientFromSecret(secret), nil
		}
		if !k8serrors.IsNotFound(loadErr) {
			return Client{}, loadErr
		}
	}
	if err != nil {
		return Client{}, err
	}
	return Client{}, nil
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

func (kubeDynamicClientStore) Save(item Client, isUpdate bool) error {
	sdk := k8s.NewK8sClient().Sdk
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	resource := oidcClientFromClient(sdk.GetNamespace(), item)
	if isUpdate {
		current := &oidcv1alpha1.OIDCClient{}
		if err := sigClient.Get(sdk.Ctx, sigclient.ObjectKey{Name: resourceNameForClientID(item.ClientID), Namespace: sdk.GetNamespace()}, current); err != nil {
			return err
		}
		resource.ResourceVersion = current.ResourceVersion
		resource.CreationTimestamp = current.CreationTimestamp
		return sigClient.Update(sdk.Ctx, resource)
	}
	return sigClient.Create(sdk.Ctx, resource)
}

func (kubeDynamicClientStore) Delete(clientID string) error {
	sdk := k8s.NewK8sClient().Sdk
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	err = sigClient.Delete(sdk.Ctx, &oidcv1alpha1.OIDCClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceNameForClientID(clientID),
			Namespace: sdk.GetNamespace(),
		},
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func clientFromOIDCClient(item *oidcv1alpha1.OIDCClient) Client {
	clientID := item.Spec.ClientID
	if clientID == "" {
		clientID = item.Name
	}
	return Client{
		Name:                  item.Spec.ClientName,
		ClientID:              clientID,
		ClientSecret:          item.Spec.ClientSecret,
		RedirectURIs:          item.Spec.RedirectURIs,
		AllowAnyRedirectURI:   item.Spec.AllowAnyRedirectURI,
		Scopes:                normalizeScopes(item.Spec.Scopes),
		TokenEndpointAuthMode: item.Spec.TokenEndpointAuthMode,
		IsDynamic:             true,
		CreatedAt:             item.CreationTimestamp.Time,
	}
}

func oidcClientFromClient(namespace string, item Client) *oidcv1alpha1.OIDCClient {
	return &oidcv1alpha1.OIDCClient{
		TypeMeta: metav1.TypeMeta{
			APIVersion: oidcv1alpha1.SchemeGroupVersion.String(),
			Kind:       "OIDCClient",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceNameForClientID(item.ClientID),
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/oidc-client": "true",
			},
		},
		Spec: oidcv1alpha1.OIDCClientSpec{
			ClientID:              item.ClientID,
			ClientName:            item.Name,
			ClientSecret:          item.ClientSecret,
			RedirectURIs:          item.RedirectURIs,
			AllowAnyRedirectURI:   item.AllowAnyRedirectURI,
			Scopes:                item.Scopes,
			TokenEndpointAuthMode: item.TokenEndpointAuthMode,
		},
	}
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
			"allow_any_redirect_uri":     []byte(boolString(client.AllowAnyRedirectURI)),
			"scopes":                     []byte(strings.Join(client.Scopes, " ")),
			"token_endpoint_auth_method": []byte(client.TokenEndpointAuthMode),
		},
	}
}

func resourceNameForClientID(clientID string) string {
	return clientID
}

func secretNameForClientID(clientID string) string {
	return resourceNameForClientID(clientID) + "-oidc"
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
	return strings.EqualFold(string(data), "true")
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
