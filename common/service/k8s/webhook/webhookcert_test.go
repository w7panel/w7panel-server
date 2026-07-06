package webhook

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

const testNamespace = "default"

func TestWebhookCertCreatesWhenSecretMissing(t *testing.T) {
	service := newTestWebhookCert()

	cert, err := service.GetCert("webhook.default.svc")
	if err != nil {
		t.Fatalf("GetCert failed: %v", err)
	}
	if cert.Cert == "" || cert.Key == "" {
		t.Fatal("expected generated certificate and key")
	}

	secret, err := service.secrets.Get(context.Background(), getSecret(), metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected secret to be created: %v", err)
	}
	if string(secret.Data["tls.crt"]) != cert.Cert {
		t.Fatal("expected returned cert to match stored secret cert")
	}
}

func TestWebhookCertReusesValidSecret(t *testing.T) {
	certPEM, keyPEM := mustTestCert(t, "webhook.default.svc", time.Now().Add(-time.Hour), time.Now().Add(90*24*time.Hour))
	service := newTestWebhookCert(withCertSecret(certPEM, keyPEM))

	cert, err := service.GetCert("webhook.default.svc")
	if err != nil {
		t.Fatalf("GetCert failed: %v", err)
	}
	if cert.Cert != string(certPEM) || cert.Key != string(keyPEM) {
		t.Fatal("expected existing valid cert to be reused")
	}
}

func TestWebhookCertRotatesExpiredSecret(t *testing.T) {
	certPEM, keyPEM := mustTestCert(t, "webhook.default.svc", time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
	service := newTestWebhookCert(withCertSecret(certPEM, keyPEM))

	cert, err := service.GetCert("webhook.default.svc")
	if err != nil {
		t.Fatalf("GetCert failed: %v", err)
	}
	if cert.Cert == string(certPEM) || cert.Key == string(keyPEM) {
		t.Fatal("expected expired cert to be rotated")
	}
	assertCertValidBeyond(t, []byte(cert.Cert), time.Now().Add(300*24*time.Hour))
}

func TestWebhookCertRotatesWhenRenewWindowReached(t *testing.T) {
	certPEM, keyPEM := mustTestCert(t, "webhook.default.svc", time.Now().Add(-time.Hour), time.Now().Add(7*24*time.Hour))
	service := newTestWebhookCert(withCertSecret(certPEM, keyPEM))

	cert, err := service.GetCert("webhook.default.svc")
	if err != nil {
		t.Fatalf("GetCert failed: %v", err)
	}
	if cert.Cert == string(certPEM) || cert.Key == string(keyPEM) {
		t.Fatal("expected near-expiring cert to be rotated")
	}
	assertCertValidBeyond(t, []byte(cert.Cert), time.Now().Add(300*24*time.Hour))
}

func TestWebhookCertRotatesInvalidSecretData(t *testing.T) {
	service := newTestWebhookCert(withCertSecret([]byte("not a certificate"), []byte("key")))

	cert, err := service.GetCert("webhook.default.svc")
	if err != nil {
		t.Fatalf("GetCert failed: %v", err)
	}
	if cert.Cert == "not a certificate" || cert.Key == "key" {
		t.Fatal("expected invalid cert data to be rotated")
	}
	assertCertValidBeyond(t, []byte(cert.Cert), time.Now().Add(300*24*time.Hour))
}

func newTestWebhookCert(objects ...*corev1.Secret) *webhookcert {
	client := fake.NewSimpleClientset(secretObjects(objects...)...)
	return &webhookcert{
		ctx:       context.Background(),
		namespace: testNamespace,
		secrets:   client.CoreV1().Secrets(testNamespace),
	}
}

func secretObjects(secrets ...*corev1.Secret) []runtime.Object {
	objects := make([]runtime.Object, 0, len(secrets))
	for _, secret := range secrets {
		objects = append(objects, secret)
	}
	return objects
}

func withCertSecret(certPEM, keyPEM []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getSecret(),
			Namespace: testNamespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": certPEM,
			"tls.key": keyPEM,
		},
	}
}

func mustTestCert(t *testing.T, host string, notBefore, notAfter time.Time) ([]byte, []byte) {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial number failed: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate failed: %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
}

func assertCertValidBeyond(t *testing.T, certPEM []byte, threshold time.Time) {
	t.Helper()

	parsed, err := parseCertificate(certPEM)
	if err != nil {
		t.Fatalf("parseCertificate failed: %v", err)
	}
	if !parsed.NotAfter.After(threshold) {
		t.Fatalf("cert NotAfter = %s, want after %s", parsed.NotAfter, threshold)
	}
}
