package appgroup

import (
	"reflect"
	"strings"

	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	networkingv1lister "k8s.io/client-go/listers/networking/v1"
)

func syncAppGroupDomains(evt *K8sResourceEvent, group *appgroupWrapper, ingressLister networkingv1lister.IngressLister) error {
	if group == nil || !group.IsExists() {
		return nil
	}
	if ingressLister == nil {
		return nil
	}
	domains, err := appGroupDomainsFromStatus(group.AppGroup.Status.Items, group.AppGroup.Namespace, ingressLister)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(group.AppGroup.GetDomains(), domains) {
		return nil
	}
	group.AppGroup.SetDomain(domains)
	group.changed = true
	return nil
}

func appGroupDomainsFromStatus(items []appv1.AppGroupItemStatus, namespace string, ingressLister networkingv1lister.IngressLister) ([]string, error) {
	domains := []string{}
	seen := map[string]struct{}{}
	for _, item := range items {
		if item.Kind != "Ingress" || item.Name == "" {
			continue
		}
		ingress, err := ingressLister.Ingresses(namespace).Get(item.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		for _, domain := range ingressDomains(ingress) {
			key := domainDedupeKey(domain)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			domains = append(domains, domain)
		}
	}
	return domains, nil
}

func ingressDomains(ingress *networkingv1.Ingress) []string {
	if ingress == nil {
		return []string{}
	}
	scheme := "http://"
	if ingress.Annotations != nil && ingress.Annotations["cert-manager.io/cluster-issuer"] == "w7-letsencrypt-prod" {
		scheme = "https://"
	}
	domains := []string{}
	for _, rule := range ingress.Spec.Rules {
		if rule.Host == "" || rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			domains = append(domains, scheme+rule.Host+normalizeIngressPath(path.Path))
		}
	}
	return domains
}

func normalizeIngressPath(path string) string {
	if path == "" || path == "/" {
		return ""
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func domainDedupeKey(domain string) string {
	domain = strings.TrimPrefix(domain, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	return strings.ToLower(domain)
}
