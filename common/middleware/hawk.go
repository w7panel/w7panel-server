package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hiyosi/hawk"
	"github.com/w7panel/w7panel/common/service/k8s"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	ranginemiddleware "github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const hawkPrefix = "Hawk "

var (
	errMissingHawkAuthorization = errors.New("missing hawk authorization header")
	errInvalidHawkAuthorization = errors.New("invalid hawk authorization header")
	errMissingHawkID            = errors.New("missing hawk id")
	errMissingHawkTimestamp     = errors.New("missing hawk ts")
	errInvalidHawkTimestamp     = errors.New("invalid hawk ts")
	errMissingHawkNonce         = errors.New("missing hawk nonce")
	errMissingHawkMAC           = errors.New("missing hawk mac")
	errInvalidHawkHash          = errors.New("invalid hawk hash")
	errHawkBodyHashMismatch     = errors.New("hawk body hash mismatch")
	errHawkSignatureMismatch    = errors.New("hawk signature mismatch")
)

type Hawk struct {
	ranginemiddleware.Abstract
	MaxSkew       time.Duration
	ResolveClient func(context.Context, string) (*apiclientv1alpha1.ApiClient, error)
}

type hawkAuthHeader struct {
	ID    string
	TS    int64
	Nonce string
	Hash  string
	Ext   string
	App   string
	Dlg   string
	MAC   string
}
type client struct {
}

func (m *client) GetCredential(id string) (*hawk.Credential, error) {
	client, err := m.resolveClient(context.Background(), id)
	if err != nil {
		return nil, err
	}
	if client.Spec.ClientSecret == "" {
		return nil, fmt.Errorf("client %s has no secret", id)
	}

	return &hawk.Credential{
		ID:  id,
		Key: client.Spec.ClientSecret,
		Alg: hawk.SHA256,
	}, nil

}

func (m *client) resolveClient(ctx context.Context, clientID string) (*apiclientv1alpha1.ApiClient, error) {
	sdk := k8s.NewK8sClient()
	k8sClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}

	var list apiclientv1alpha1.ApiClientList
	if err := k8sClient.List(ctx, &list, sigclient.InNamespace(sdk.GetNamespace())); err != nil {
		return nil, err
	}

	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.ClientID == clientID {
			return item, nil
		}
	}

	return nil, errClientNotFound
}

func (m Hawk) Process(ctx *gin.Context) {
	if ctx.Request.Method == http.MethodOptions {
		ctx.Next()
		return
	}
	s := hawk.NewServer(&client{})
	// authenticate client request
	cred, err := s.Authenticate(ctx.Request)
	if err != nil {
		ctx.AbortWithError(http.StatusUnauthorized, err)
		return
	}

	ctx.Set("api_client_id", cred.ID)
	ctx.Next()
}

func (m Hawk) maxSkew() time.Duration {
	if m.MaxSkew <= 0 {
		return 5 * time.Minute
	}
	return m.MaxSkew
}
