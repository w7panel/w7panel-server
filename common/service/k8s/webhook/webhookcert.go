package webhook

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	CertDir                 = "/tmp/k8s-webhook-server/serving-certs"
	certFile                = "tls.crt"
	keyFile                 = "tls.key"
	certValidityDuration    = 365 * 24 * time.Hour
	certRenewBeforeDuration = 30 * 24 * time.Hour
)
const CERT_SECRET_NAME = "webhook-cert"

func ensureCertificates(namespace string) error {
	sdk := k8s.NewK8sClient().Sdk
	cert, err := NewCert(sdk).GetCert(getSvcHost(sdk.GetNamespace()))
	if err != nil {
		return err
	}
	return cert.WriteToFile()
}

func Cert(namespace string) error {
	return ensureCertificates(namespace)
}

type cert struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

func (c *cert) ToSecret() corev1.Secret {
	return corev1.Secret{
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": []byte(c.Cert),
			"tls.key": []byte(c.Key),
		},
	}
}
func (c *cert) WriteToFile() error {
	// Create cert directory if not exists
	if err := os.MkdirAll(CertDir, 0755); err != nil {
		return err
	}

	// Write cert file
	certPath := filepath.Join(CertDir, certFile)
	if err := os.WriteFile(certPath, []byte(c.Cert), 0644); err != nil {
		return err
	}

	// Write key file
	keyPath := filepath.Join(CertDir, keyFile)
	if err := os.WriteFile(keyPath, []byte(c.Key), 0600); err != nil {
		return err
	}

	return nil
}

type webhookcert struct {
	ctx       context.Context
	namespace string
	secrets   typedcorev1.SecretInterface
}

func NewCert(sdk *k8s.Sdk) *webhookcert {
	namespace := sdk.GetNamespace()
	return &webhookcert{
		ctx:       sdk.Ctx,
		namespace: namespace,
		secrets:   sdk.ClientSet.CoreV1().Secrets(namespace),
	}
}

func (c *webhookcert) GetCert(host string) (*cert, error) {
	secret, err := c.secrets.Get(c.ctx, getSecret(), metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return c.CreateCert(host)
		}
		return nil, err
	}
	certPEM := secret.Data["tls.crt"]
	keyPEM := secret.Data["tls.key"]
	if !shouldReuseCert(certPEM, keyPEM, time.Now()) {
		return c.CreateCert(host)
	}

	return &cert{
		Cert: string(certPEM),
		Key:  string(keyPEM),
	}, nil
}

func shouldReuseCert(certPEM, keyPEM []byte, now time.Time) bool {
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return false
	}
	parsed, err := parseCertificate(certPEM)
	if err != nil {
		return false
	}
	return parsed.NotAfter.After(now.Add(certRenewBeforeDuration))
}

func parseCertificate(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("decode certificate pem")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("unexpected certificate pem type %q", block.Type)
	}
	return x509.ParseCertificate(block.Bytes)
}

func (c *webhookcert) CreateCert(host string) (*cert, error) {
	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(certValidityDuration),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}

	// Create self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}

	// Encode certificate and key to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: derBytes,
	})

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	// Create secret with certificate
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getSecret(),
			Namespace: c.namespace,
		},
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": privPEM,
		},
	}

	// Create or update secret
	_, err = c.secrets.Create(c.ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = c.secrets.Update(c.ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	return &cert{
		Cert: string(certPEM),
		Key:  string(privPEM),
	}, nil
}
