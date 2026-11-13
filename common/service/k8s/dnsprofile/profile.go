package dnsprofile

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	secretNamespace   = "cert-manager"
	profilePrefix     = "w7-dns-"
	grantKeyName      = "w7-dns-grant-key"
	ProfileAnnotation = "w7.cc/dns-profile"
	GrantAnnotation   = "w7.cc/dns-grant"
)

var issuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "clusterissuers"}

type Profile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Zone     string `json:"zone"`
	Issuer   string `json:"issuer"`
}

type Store struct{ SDK *k8s.Sdk }

func ownerLabel(owner string) string {
	sum := sha256.Sum256([]byte(owner))
	return hex.EncodeToString(sum[:12])
}

func profileFromSecret(secret *corev1.Secret) Profile {
	return Profile{
		ID:       strings.TrimPrefix(secret.Name, profilePrefix),
		Name:     secret.Annotations["w7.cc/dns-name"],
		Provider: secret.Annotations["w7.cc/dns-provider"],
		Zone:     secret.Annotations["w7.cc/dns-zone"],
		Issuer:   secret.Name,
	}
}

func (s Store) List(ctx context.Context, owner string) ([]Profile, error) {
	list, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "w7.cc/dns-profile=true,w7.cc/dns-owner=" + ownerLabel(owner),
	})
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(list.Items))
	for i := range list.Items {
		profiles = append(profiles, profileFromSecret(&list.Items[i]))
	}
	return profiles, nil
}

func (s Store) Get(ctx context.Context, owner, id string) (Profile, error) {
	if !validID(id) {
		return Profile{}, errors.New("DNS 配置不存在")
	}
	secret, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Get(ctx, profilePrefix+id, metav1.GetOptions{})
	if err != nil {
		return Profile{}, err
	}
	if secret.Labels["w7.cc/dns-profile"] != "true" || secret.Labels["w7.cc/dns-owner"] != ownerLabel(owner) {
		return Profile{}, errors.New("无权使用该 DNS 配置")
	}
	return profileFromSecret(secret), nil
}

func validID(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s Store) Create(ctx context.Context, owner, name, provider, zone string, values map[string]string) (Profile, error) {
	name = strings.TrimSpace(name)
	if name != "" && len([]rune(name)) > 80 {
		return Profile{}, errors.New("DNS 配置名称长度必须为 1 到 80 个字符")
	}
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	credentials, err := ProviderCredentials(provider, values)
	if err != nil {
		return Profile{}, err
	}
	if err := Verify(provider, zone, credentials); err != nil {
		return Profile{}, err
	}

	idBytes := make([]byte, 12)
	if _, err := rand.Read(idBytes); err != nil {
		return Profile{}, err
	}
	id := hex.EncodeToString(idBytes)
	if name == "" {
		name = "*." + zone + "-" + id[:8]
	}
	secretName := profilePrefix + id
	data := make(map[string][]byte, len(credentials))
	for key, value := range credentials {
		data[key] = []byte(value)
	}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name: secretName, Namespace: secretNamespace,
		Labels:      map[string]string{"w7.cc/dns-profile": "true", "w7.cc/dns-owner": ownerLabel(owner)},
		Annotations: map[string]string{"w7.cc/dns-name": name, "w7.cc/dns-provider": provider, "w7.cc/dns-zone": zone},
	}, Type: corev1.SecretTypeOpaque, Data: data}
	if _, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		return Profile{}, err
	}
	if err := s.createIssuer(ctx, secretName, provider); err != nil {
		_ = s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Delete(ctx, secretName, metav1.DeleteOptions{})
		return Profile{}, err
	}
	return Profile{ID: id, Name: name, Provider: provider, Zone: zone, Issuer: secret.Name}, nil
}

func (s Store) createIssuer(ctx context.Context, name, provider string) error {
	current, err := s.SDK.DynamicClient().Resource(issuerGVR).Get(ctx, "w7-letsencrypt-prod", metav1.GetOptions{})
	if err != nil {
		return err
	}
	acme, ok, err := unstructured.NestedMap(current.Object, "spec", "acme")
	if err != nil || !ok {
		return errors.New("现有 Let's Encrypt 签发器缺少 ACME 配置")
	}
	server, _, _ := unstructured.NestedString(acme, "server")
	email, _, _ := unstructured.NestedString(acme, "email")
	if server == "" || email == "" {
		return errors.New("现有 Let's Encrypt 签发器配置不完整")
	}
	issuer := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "ClusterIssuer",
		"metadata": map[string]any{"name": name, "labels": map[string]any{"w7.cc/dns-profile": "true"}},
		"spec": map[string]any{"acme": map[string]any{
			"server": server, "email": email,
			"privateKeySecretRef": map[string]any{"name": name + "-account"},
			"solvers": []any{map[string]any{"dns01": map[string]any{"webhook": map[string]any{
				"groupName": "lego.dns-solver", "solverName": "lego-solver",
				"config": map[string]any{"provider": provider, "envFrom": map[string]any{"secret": map[string]any{
					"name": name, "namespace": secretNamespace,
				}}},
			}}}},
		}},
	}}
	_, err = s.SDK.DynamicClient().Resource(issuerGVR).Create(ctx, issuer, metav1.CreateOptions{})
	return err
}

func WildcardZone(host string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if !strings.HasPrefix(host, "*.") || !validZone(host[2:]) {
		return "", errors.New("泛域名无效")
	}
	return host[2:], nil
}

func WildcardInZone(host, zone string) bool {
	base, err := WildcardZone(host)
	if err != nil {
		return false
	}
	zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	return base == zone || strings.HasSuffix(base, "."+zone)
}

func (s Store) Delete(ctx context.Context, owner, id string) error {
	profile, err := s.Get(ctx, owner, id)
	if err != nil {
		return err
	}
	ingresses, err := s.SDK.ClientSet.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, ingress := range ingresses.Items {
		if ingress.Annotations["cert-manager.io/cluster-issuer"] == profile.Issuer {
			return errors.New("DNS 配置正被域名使用，不能删除")
		}
	}
	certs, err := s.SDK.DynamicClient().Resource(schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cert := range certs.Items {
		issuer, _, _ := unstructured.NestedString(cert.Object, "spec", "issuerRef", "name")
		if issuer == profile.Issuer {
			return errors.New("DNS 配置正被证书使用，不能删除")
		}
	}
	if err := s.SDK.DynamicClient().Resource(issuerGVR).Delete(ctx, profile.Issuer, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Delete(ctx, profile.Issuer, metav1.DeleteOptions{})
}

func (s Store) Grant(ctx context.Context, owner, id, namespace, name, host string) (map[string]string, error) {
	profile, err := s.Get(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if !WildcardInZone(host, profile.Zone) || namespace == "" || name == "" {
		return nil, errors.New("泛域名、命名空间或 Ingress 名称无效")
	}
	key, err := s.grantKey(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"cert-manager.io/cluster-issuer": profile.Issuer,
		ProfileAnnotation:                profile.ID,
		GrantAnnotation:                  signGrant(key, namespace, name, host, profile.Issuer),
	}, nil
}

func signGrant(key []byte, namespace, name, host, issuer string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(strings.Join([]string{namespace, name, strings.ToLower(host), issuer}, "\n")))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s Store) grantKey(ctx context.Context) ([]byte, error) {
	secret, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Get(ctx, grantKeyName, metav1.GetOptions{})
	if err == nil {
		if len(secret.Data["key"]) != 32 {
			return nil, errors.New("DNS 授权密钥无效")
		}
		return secret.Data["key"], nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	secret = &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: grantKeyName, Namespace: secretNamespace},
		Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"key": key}}
	if _, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		return s.grantKey(ctx)
	}
	return key, nil
}

func (s Store) CheckIngressGrant(ctx context.Context, namespace, name, host, issuer, id, signature string) error {
	if !strings.HasPrefix(issuer, profilePrefix) {
		return nil
	}
	if !validID(id) || issuer != profilePrefix+id {
		return errors.New("DNS 签发器与配置不匹配")
	}
	secret, err := s.SDK.ClientSet.CoreV1().Secrets(secretNamespace).Get(ctx, issuer, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if !WildcardInZone(host, secret.Annotations["w7.cc/dns-zone"]) {
		return errors.New("泛域名不属于已验证的 DNS 区域")
	}
	key, err := s.grantKey(ctx)
	if err != nil {
		return err
	}
	expected, err := hex.DecodeString(signGrant(key, namespace, name, host, issuer))
	if err != nil {
		return err
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(expected, provided) {
		return fmt.Errorf("DNS 签发授权无效")
	}
	return nil
}

// DeriveChildGrant permits the ingress synchronizer to copy a verified child binding to its generated routes.
func DeriveChildGrant(ctx context.Context, c client.Client, namespace, authorizedName, targetName, host, id, signature string) (map[string]string, error) {
	if !validID(id) {
		return nil, errors.New("DNS 配置不存在")
	}
	issuer := profilePrefix + id
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: secretNamespace, Name: issuer}, secret); err != nil {
		return nil, err
	}
	if secret.Labels["w7.cc/dns-profile"] != "true" || !WildcardInZone(host, secret.Annotations["w7.cc/dns-zone"]) {
		return nil, errors.New("泛域名不属于已验证的 DNS 区域")
	}
	keySecret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: secretNamespace, Name: grantKeyName}, keySecret); err != nil {
		return nil, err
	}
	if len(keySecret.Data["key"]) != 32 {
		return nil, errors.New("DNS 授权密钥无效")
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return nil, errors.New("DNS 签发授权无效")
	}
	expected, _ := hex.DecodeString(signGrant(keySecret.Data["key"], namespace, authorizedName, host, issuer))
	if !hmac.Equal(expected, provided) {
		return nil, errors.New("DNS 签发授权无效")
	}
	return map[string]string{
		"cert-manager.io/cluster-issuer": issuer,
		ProfileAnnotation:                id,
		GrantAnnotation:                  signGrant(keySecret.Data["key"], namespace, targetName, host, issuer),
	}, nil
}
