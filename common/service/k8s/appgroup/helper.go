package appgroup

import (
	"errors"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func AppGroupToOidcSecret(name string) (*corev1.Secret, error) {
	group, err := GetAppgroupUseSdk(name, "default", k8s.NewK8sClient().Sdk)
	if err != nil {
		return nil, err
	}

	redirectURIs := group.GetDomains()
	if defaultDomain := strings.TrimSpace(group.GetDefaultDomain()); defaultDomain != "" {
		found := false
		for _, item := range redirectURIs {
			if item == defaultDomain {
				found = true
				break
			}
		}
		if !found {
			redirectURIs = append(redirectURIs, defaultDomain)
		}
	}
	if len(redirectURIs) == 0 {
		return nil, errors.New("appgroup missing oidc redirect uris")
	}

	clientName := strings.TrimSpace(group.Spec.Title)
	if clientName == "" {
		clientName = group.Name
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      group.Name + "-oidc",
			Namespace: group.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"client_id":                  []byte(group.Name),
			"client_name":                []byte(clientName),
			"client_secret":              []byte(""),
			"redirect_uris":              []byte(strings.Join(redirectURIs, "\n")),
			"scopes":                     []byte("openid profile offline_access"),
			"token_endpoint_auth_method": []byte("none"),
		},
	}, nil
}
