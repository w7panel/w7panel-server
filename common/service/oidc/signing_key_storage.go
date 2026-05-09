package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	oidcSigningKeyNamespace = "kube-system"
	oidcSigningKeySecret    = "w7-oidc"
	oidcSigningKeyField     = "signing_key.pem"
)

func loadOrCreateSigningKey(fallbackPEM string) (*rsa.PrivateKey, error) {
	sdk := k8s.NewK8sClient().Sdk
	secret, err := sdk.ClientSet.CoreV1().Secrets(oidcSigningKeyNamespace).Get(sdk.Ctx, oidcSigningKeySecret, metav1.GetOptions{})
	if err == nil {
		return parseSigningKeySecret(secret)
	}
	if !k8serrors.IsNotFound(err) {
		return nil, err
	}

	privateKey, pemValue, err := buildInitialSigningKey(fallbackPEM)
	if err != nil {
		return nil, err
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      oidcSigningKeySecret,
			Namespace: oidcSigningKeyNamespace,
			Labels: map[string]string{
				"w7.cc/oidc": "true",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			oidcSigningKeyField: []byte(pemValue),
		},
	}
	if _, err := sdk.ClientSet.CoreV1().Secrets(oidcSigningKeyNamespace).Create(sdk.Ctx, secret, metav1.CreateOptions{}); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return nil, err
		}
		current, getErr := sdk.ClientSet.CoreV1().Secrets(oidcSigningKeyNamespace).Get(sdk.Ctx, oidcSigningKeySecret, metav1.GetOptions{})
		if getErr != nil {
			return nil, getErr
		}
		return parseSigningKeySecret(current)
	}
	return privateKey, nil
}

func buildInitialSigningKey(fallbackPEM string) (*rsa.PrivateKey, string, error) {
	if strings.TrimSpace(fallbackPEM) != "" {
		privateKey, err := parsePrivateKeyPEM(fallbackPEM)
		if err != nil {
			return nil, "", err
		}
		return privateKey, normalizePrivateKeyPEM(privateKey), nil
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", err
	}
	return privateKey, normalizePrivateKeyPEM(privateKey), nil
}

func parseSigningKeySecret(secret *corev1.Secret) (*rsa.PrivateKey, error) {
	if secret == nil {
		return nil, errors.New("oidc signing key secret is nil")
	}
	pemValue := strings.TrimSpace(string(secret.Data[oidcSigningKeyField]))
	if pemValue == "" {
		return nil, errors.New("oidc signing key secret missing signing_key.pem")
	}
	return parsePrivateKeyPEM(pemValue)
}

func parsePrivateKeyPEM(pemValue string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemValue))
	if block == nil {
		return nil, errors.New("invalid oidc signing key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("oidc signing key must be RSA private key")
	}
	return key, nil
}

func normalizePrivateKeyPEM(privateKey *rsa.PrivateKey) string {
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	return string(pem.EncodeToMemory(block))
}
