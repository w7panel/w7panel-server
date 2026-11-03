package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s/dnsprofile"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type managedCertificate struct {
	Metadata metav1.ObjectMeta `json:"metadata"`
	Spec     struct {
		DNSNames   []string `json:"dnsNames"`
		CommonName string   `json:"commonName"`
		SecretName string   `json:"secretName"`
		IssuerRef  struct {
			Name string `json:"name"`
		} `json:"issuerRef"`
	} `json:"spec"`
}

func (m *ResourceMutator) handleCertificate(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation == "DELETE" {
		return admission.Allowed("删除证书")
	}
	var certificate managedCertificate
	if err := json.Unmarshal(req.Object.Raw, &certificate); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	issuer := certificate.Spec.IssuerRef.Name
	if !strings.HasPrefix(issuer, "w7-dns-") {
		return admission.Allowed("非托管 DNS 签发器")
	}
	if len(certificate.Spec.DNSNames) != 1 ||
		(certificate.Spec.CommonName != "" && certificate.Spec.CommonName != certificate.Spec.DNSNames[0]) {
		return admission.Denied("DNS 证书只能签发已授权的单个泛域名")
	}
	namespace := certificate.Metadata.Namespace
	if namespace == "" {
		namespace = req.Namespace
	}
	for _, owner := range certificate.Metadata.OwnerReferences {
		if owner.Kind != "Ingress" || owner.APIVersion != "networking.k8s.io/v1" {
			continue
		}
		ingress := &networkingv1.Ingress{}
		if err := m.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: owner.Name}, ingress); err != nil {
			return admission.Denied("找不到 DNS 证书关联的 Ingress")
		}
		if ingress.UID != owner.UID || ingress.Annotations["cert-manager.io/cluster-issuer"] != issuer {
			return admission.Denied("DNS 证书的 Ingress 授权不匹配")
		}
		for _, tls := range ingress.Spec.TLS {
			if tls.SecretName != certificate.Spec.SecretName {
				continue
			}
			for _, host := range tls.Hosts {
				if host != certificate.Spec.DNSNames[0] {
					continue
				}
				if err := (dnsprofile.Store{SDK: m.sdk}).CheckIngressGrant(ctx, namespace, ingress.Name, host, issuer,
					ingress.Annotations[dnsprofile.ProfileAnnotation], ingress.Annotations[dnsprofile.GrantAnnotation]); err != nil {
					return admission.Denied(err.Error())
				}
				return admission.Allowed("DNS 证书已有 Ingress 授权")
			}
		}
	}
	return admission.Denied("DNS 证书必须来自已授权的 Ingress")
}
