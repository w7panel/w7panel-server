package oidc

import (
	"context"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func panelIngressIssuer(ctx context.Context, client kubernetes.Interface, namespace, deployment string) (string, error) {
	if namespace == "" || deployment == "" {
		return "", nil
	}
	ingresses, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=" + deployment})
	if err != nil {
		return "", err
	}
	for _, ingress := range ingresses.Items {
		if ingress.Labels["parents"] != "" || len(ingress.Spec.Rules) == 0 {
			continue
		}
		host := ingress.Spec.Rules[0].Host
		if host == "" || strings.HasPrefix(host, "*.") {
			continue
		}
		scheme := "http://"
		for _, tls := range ingress.Spec.TLS {
			for _, tlsHost := range tls.Hosts {
				if tlsHost == host {
					scheme = "https://"
				}
			}
		}
		return scheme + host + oidcBasePath, nil
	}
	return "", nil
}
