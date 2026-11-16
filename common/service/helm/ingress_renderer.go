package helm

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s/dnsprofile"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlreader "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// Certificate defaults and DNS grants use the resources Helm actually renders,
// including third-party names, fullnameOverride and dependency Chart resources.
type ingressCertificateRenderer struct {
	namespace, releaseName string
	grants                 []dnsprofile.HelmGrant
	autoSSLHosts           []string
	derive                 func(namespace, authorizedName, targetName, host, id, signature string) (map[string]string, error)
	existing               func(namespace, name string) (*networkingv1.Ingress, error)
}

func (r *ingressCertificateRenderer) Run(input *bytes.Buffer) (*bytes.Buffer, error) {
	reader := yamlreader.NewYAMLReader(bufio.NewReader(bytes.NewReader(input.Bytes())))
	output := &bytes.Buffer{}
	for {
		document, err := reader.Read()
		if err == io.EOF {
			return output, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read Helm Ingress manifest: %w", err)
		}
		rendered, err := r.renderDocument(document)
		if err != nil {
			return nil, err
		}
		output.WriteString("---\n")
		output.Write(rendered)
		if len(rendered) > 0 && rendered[len(rendered)-1] != '\n' {
			output.WriteByte('\n')
		}
	}
}

func (r *ingressCertificateRenderer) renderDocument(document []byte) ([]byte, error) {
	object := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(document, &object.Object); err != nil {
		return nil, err
	}
	if object.GetKind() != "Ingress" || !strings.HasPrefix(object.GetAPIVersion(), "networking.k8s.io/") {
		return document, nil
	}
	tls, _, err := unstructured.NestedSlice(object.Object, "spec", "tls")
	if err != nil {
		return nil, err
	}
	if len(tls) == 0 {
		return document, nil
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	namespace := object.GetNamespace()
	if namespace == "" {
		namespace = r.namespace
	}
	// A Chart's explicit external issuer remains entirely owned by the Chart.
	issuer := annotations["cert-manager.io/cluster-issuer"]
	if annotations["cert-manager.io/issuer"] != "" || (issuer != "" && issuer != "w7-letsencrypt-prod" && !strings.HasPrefix(issuer, "w7-dns-")) {
		return document, nil
	}
	autoSSL := issuer == "w7-letsencrypt-prod" || strings.HasPrefix(issuer, "w7-dns-") || annotations[dnsprofile.ProfileAnnotation] != ""
	hosts := map[string]bool{}
	wildcard := ""
	multipleWildcards := false
	for _, entry := range tls {
		config, ok := entry.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("Ingress %s has invalid TLS configuration", object.GetName())
		}
		tlsHosts, _, err := unstructured.NestedStringSlice(config, "hosts")
		if err != nil {
			return nil, err
		}
		for _, host := range tlsHosts {
			if strings.HasSuffix(host, ".invalid") {
				continue
			}
			hosts[host] = true
			for _, selected := range r.autoSSLHosts {
				if selected == host {
					autoSSL = true
				}
			}
			for _, grant := range r.grants {
				if grant.Host == host {
					autoSSL = true
				}
			}
			if !strings.HasPrefix(host, "*.") {
				continue
			}
			if wildcard != "" && wildcard != host {
				multipleWildcards = true
			}
			wildcard = host
		}
	}
	if !autoSSL && r.existing != nil {
		current, err := r.existing(namespace, object.GetName())
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
		if err == nil && (current.Annotations["cert-manager.io/cluster-issuer"] == "w7-letsencrypt-prod" || current.Annotations[dnsprofile.ProfileAnnotation] != "") {
			// A live domain edited through the panel may differ from saved values.
			// Only recognize previously managed certificates for the same host.
			for _, entry := range current.Spec.TLS {
				for _, host := range entry.Hosts {
					if hosts[host] {
						autoSSL = true
					}
				}
			}
		}
	}
	if !autoSSL || len(hosts) == 0 {
		return document, nil
	}
	if multipleWildcards {
		return nil, fmt.Errorf("Ingress %s 的多个泛域名需要分别配置资源", object.GetName())
	}
	if wildcard != "" {
		grant, err := r.wildcardGrant(namespace, object.GetName(), wildcard, annotations)
		if err != nil {
			return nil, fmt.Errorf("Ingress %s/%s DNS 授权失败: %w", namespace, object.GetName(), err)
		}
		for key, value := range grant {
			annotations[key] = value
		}
	}
	// Respect a Chart's explicit issuer; the panel only supplies defaults.
	if annotations["cert-manager.io/cluster-issuer"] == "" && annotations["cert-manager.io/issuer"] == "" {
		annotations["cert-manager.io/cluster-issuer"] = "w7-letsencrypt-prod"
	}
	if annotations["cert-manager.io/renew-before"] == "" {
		annotations["cert-manager.io/renew-before"] = "30m"
	}
	object.SetAnnotations(annotations)
	return yaml.Marshal(object.Object)
}

func (r *ingressCertificateRenderer) wildcardGrant(namespace, name, host string, annotations map[string]string) (map[string]string, error) {
	var selected *dnsprofile.HelmGrant
	for i := range r.grants {
		grant := &r.grants[i]
		if grant.Host != host {
			continue
		}
		if selected != nil && selected.ProfileID != grant.ProfileID {
			return nil, fmt.Errorf("同一泛域名使用了不同 DNS 配置")
		}
		selected = grant
	}
	if selected != nil {
		return r.derive(namespace, dnsprofile.HelmGrantScope(r.releaseName), name, host, selected.ProfileID, selected.Signature)
	}
	if annotations[dnsprofile.ProfileAnnotation] != "" {
		return r.derive(namespace, name, name, host, annotations[dnsprofile.ProfileAnnotation], annotations[dnsprofile.GrantAnnotation])
	}
	if r.existing == nil {
		return nil, fmt.Errorf("泛域名自动 HTTPS 需要已验证的 DNS 授权")
	}
	current, err := r.existing(namespace, name)
	if apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("泛域名自动 HTTPS 需要已验证的 DNS 授权")
	}
	if err != nil {
		return nil, err
	}
	return r.derive(namespace, name, name, host, current.Annotations[dnsprofile.ProfileAnnotation], current.Annotations[dnsprofile.GrantAnnotation])
}
