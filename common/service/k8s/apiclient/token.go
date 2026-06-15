package apiclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultTemporaryTokenMinutes = int64(10)
	MinTemporaryTokenMinutes     = int64(1)
	MaxTemporaryTokenMinutes     = int64(1440)
	permanentTokenSecretPrefix   = "w7panel-api-token-"
)

var (
	ErrInvalidCredentials = errors.New("appid or appsecret is invalid")
	ErrClientDisabled     = errors.New("api client is disabled")
)

type ExchangeTokenResult struct {
	Token     string
	TokenType string
	ExpiresIn int64
}

func ExchangeToken(ctx context.Context, namespace, appID, appSecret string) (*ExchangeTokenResult, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" || appSecret == "" {
		return nil, ErrInvalidCredentials
	}

	sdk := k8s.NewK8sClient().Sdk
	client, err := GetCachedApiClientByID(ctx, namespace, appID, loadApiClients)
	if err != nil {
		return nil, err
	}
	if client == nil || client.Spec.ClientID != appID {
		return nil, ErrInvalidCredentials
	}
	if client.Spec.Enabled != nil && !*client.Spec.Enabled {
		return nil, ErrClientDisabled
	}
	if client.Spec.ClientSecret == "" || client.Spec.ClientSecret != appSecret {
		return nil, ErrInvalidCredentials
	}

	tokenType := NormalizeTokenType(client.Spec.TokenType)
	switch tokenType {
	case apiclientv1alpha1.TokenTypeTemporary:
		expiresIn := TemporaryTokenSeconds(client.Spec.TemporaryTokenMinutes)
		token, err := sdk.CreateTokenRequest(permissionservice.APIPermissionName, expiresIn, []string{})
		if err != nil {
			return nil, err
		}
		MarkAccessed(client.Namespace, client.Name, time.Now())
		return &ExchangeTokenResult{Token: token, TokenType: tokenType, ExpiresIn: expiresIn}, nil
	case apiclientv1alpha1.TokenTypePermanent:
		token, err := permanentToken(ctx, sdk, client)
		if err != nil {
			return nil, err
		}
		MarkAccessed(client.Namespace, client.Name, time.Now())
		return &ExchangeTokenResult{Token: token, TokenType: tokenType, ExpiresIn: 0}, nil
	default:
		return nil, fmt.Errorf("unsupported token type %q", tokenType)
	}
}

func NormalizeTokenType(tokenType string) string {
	switch tokenType {
	case apiclientv1alpha1.TokenTypePermanent:
		return apiclientv1alpha1.TokenTypePermanent
	default:
		return apiclientv1alpha1.TokenTypeTemporary
	}
}

func IsValidTokenType(tokenType string) bool {
	return tokenType == apiclientv1alpha1.TokenTypeTemporary || tokenType == apiclientv1alpha1.TokenTypePermanent
}

func TemporaryTokenMinutes(minutes *int64) int64 {
	if minutes == nil {
		return DefaultTemporaryTokenMinutes
	}
	if *minutes < MinTemporaryTokenMinutes {
		return MinTemporaryTokenMinutes
	}
	if *minutes > MaxTemporaryTokenMinutes {
		return MaxTemporaryTokenMinutes
	}
	return *minutes
}

func TemporaryTokenSeconds(minutes *int64) int64 {
	return TemporaryTokenMinutes(minutes) * 60
}

func loadApiClients(ctx context.Context, namespace string) ([]apiclientv1alpha1.ApiClient, error) {
	k8sClient, err := k8s.NewK8sClient().Sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	list := &apiclientv1alpha1.ApiClientList{}
	if err := k8sClient.List(ctx, list, ctrlclient.InNamespace(namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func permanentToken(ctx context.Context, sdk *k8s.Sdk, client *apiclientv1alpha1.ApiClient) (string, error) {
	secretName := permanentTokenSecretName(client.Spec.ClientID)
	secrets := sdk.ClientSet.CoreV1().Secrets(sdk.GetNamespace())
	secret, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: sdk.GetNamespace(),
				Labels: map[string]string{
					"w7.cc/api-token": "true",
					"w7.cc/appid":     client.Spec.ClientID,
				},
				Annotations: map[string]string{
					corev1.ServiceAccountNameKey: permissionservice.APIPermissionName,
				},
			},
			Type: corev1.SecretTypeServiceAccountToken,
		}
		secret, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
	}
	if err != nil {
		return "", err
	}
	if token := string(secret.Data[corev1.ServiceAccountTokenKey]); token != "" {
		return token, nil
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		secret, err = secrets.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		if token := string(secret.Data[corev1.ServiceAccountTokenKey]); token != "" {
			return token, nil
		}
	}
	return "", errors.New("service account token secret is not ready")
}

func permanentTokenSecretName(appID string) string {
	sum := sha256.Sum256([]byte(appID))
	return permanentTokenSecretPrefix + hex.EncodeToString(sum[:])[:16]
}
