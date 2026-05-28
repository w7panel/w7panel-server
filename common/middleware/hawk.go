package middleware

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	k8sapiclient "github.com/w7panel/w7panel/common/service/k8s/apiclient"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	ranginemiddleware "github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	"go.mozilla.org/hawk"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	errInvalidHawkAuthorization = errors.New("invalid hawk authorization header")
	hawkValidationMu            sync.Mutex
)

type Hawk struct {
	ranginemiddleware.Abstract
	MaxSkew       time.Duration
	ResolveClient func(context.Context, string) (*apiclientv1alpha1.ApiClient, error)
}

type client struct {
	ResolveClient func(context.Context, string) (*apiclientv1alpha1.ApiClient, error)
	Namespace     string
}

func (m *client) resolveClient(ctx context.Context, clientID string) (*apiclientv1alpha1.ApiClient, error) {
	namespace := m.Namespace
	if namespace == "" {
		namespace = k8s.NewK8sClient().GetNamespace()
	}

	return k8sapiclient.GetCachedApiClientByID(ctx, namespace, clientID, func(ctx context.Context, namespace string) ([]apiclientv1alpha1.ApiClient, error) {
		if m.ResolveClient != nil {
			client, err := m.ResolveClient(ctx, clientID)
			if err != nil {
				return nil, err
			}
			return []apiclientv1alpha1.ApiClient{*client.DeepCopy()}, nil
		}

		sdk := k8s.NewK8sClient()
		k8sClient, err := sdk.ToSigClient()
		if err != nil {
			return nil, err
		}

		var list apiclientv1alpha1.ApiClientList
		if err := k8sClient.List(ctx, &list, sigclient.InNamespace(namespace)); err != nil {
			return nil, err
		}

		return list.Items, nil
	})
}

func (m Hawk) Process(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions {
		ctx.Next()
		return
	}

	resolver := &client{
		ResolveClient: m.ResolveClient,
		Namespace:     k8s.NewK8sClient().GetNamespace(),
	}
	apiClient, auth, err := authenticateHawkRequest(ctx.Request, resolver, m.maxSkew())
	if err != nil {
		ctx.AbortWithError(http.StatusUnauthorized, err)
		return
	}

	k8sapiclient.MarkAccessed(apiClient.Namespace, apiClient.Name, time.Now().UTC())
	ctx.Set("api_client_id", apiClient.Spec.ClientID)
	ctx.Set("api_client_name", apiClient.Spec.ClientName)
	ctx.Set("hawk_client_id", auth.Credentials.ID)
	ctx.Next()
}

func (m Hawk) maxSkew() time.Duration {
	if m.MaxSkew <= 0 {
		return 5 * time.Minute
	}
	return m.MaxSkew
}

func authenticateHawkRequest(req *http.Request, resolver *client, maxSkew time.Duration) (*apiclientv1alpha1.ApiClient, *hawk.Auth, error) {
	hawkValidationMu.Lock()
	defer hawkValidationMu.Unlock()

	originalSkew := hawk.MaxTimestampSkew
	hawk.MaxTimestampSkew = maxSkew
	defer func() {
		hawk.MaxTimestampSkew = originalSkew
	}()

	auth, err := hawk.NewAuthFromRequest(req, resolver.lookupCredentials, nil)
	if err != nil {
		return nil, nil, err
	}
	if err := auth.Valid(); err != nil {
		return nil, nil, err
	}

	apiClient, _ := auth.Credentials.Data.(*apiclientv1alpha1.ApiClient)
	if apiClient == nil {
		return nil, nil, errClientNotFound
	}
	return apiClient.DeepCopy(), auth, nil
}

func (m *client) lookupCredentials(creds *hawk.Credentials) error {
	if creds == nil || creds.ID == "" {
		return errInvalidHawkAuthorization
	}

	apiClient, err := m.resolveClient(context.Background(), creds.ID)
	if err != nil {
		return err
	}
	if apiClient == nil {
		return errClientNotFound
	}
	if apiClient.Spec.ClientSecret == "" {
		return errInvalidHawkAuthorization
	}

	creds.Key = apiClient.Spec.ClientSecret
	creds.Data = apiClient.DeepCopy()
	return nil
}
