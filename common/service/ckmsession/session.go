// Package ckmsession binds an embedded panel session to one CKM object.
package ckmsession

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/panelauth"
	userservice "github.com/w7panel/w7panel/common/service/user"
	authv1 "k8s.io/api/authentication/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var ErrDenied = errors.New("CKM panel access denied")

func loadTarget(ctx context.Context, client dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	if namespace == "" || name == "" || !strings.HasPrefix(namespace, "k3k-") {
		return nil, ErrDenied
	}
	for _, version := range []string{"v1alpha2", "v1alpha1"} {
		obj, err := client.Resource(schema.GroupVersionResource{Group: "ckm.w7.cc", Version: version, Resource: "ckms"}).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			if obj.GetUID() == "" || obj.GetDeletionTimestamp() != nil {
				return nil, ErrDenied
			}
			return obj, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	return nil, ErrDenied
}

func authorizeActor(ctx context.Context, sdk *k8s.Sdk, actor, namespace string) error {
	u, err := userservice.Get(ctx, sdk, actor)
	if err != nil {
		return ErrDenied
	}
	role := u.Spec.Role
	if role == "" {
		role = u.Spec.UserMode
	}
	if role != "founder" && namespace != "k3k-"+actor {
		return ErrDenied
	}
	return nil
}

func Audiences(p panelauth.Principal) []string {
	return []string{p.Username, "normal", "", p.CVMName, p.K3KNamespace, "https://kubernetes.default.svc.cluster.local", "k3s"}
}

// validateBootstrap is called only after Kubernetes has authenticated the token.
func validateBootstrap(raw, reviewedSubject, saNamespace string, p panelauth.Principal, now time.Time) (time.Duration, error) {
	if reviewedSubject != "system:serviceaccount:"+saNamespace+":normal" {
		return 0, ErrDenied
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(raw, claims); err != nil {
		return 0, ErrDenied
	}
	aud, err := claims.GetAudience()
	if err != nil || strings.Join(aud, "\x00") != strings.Join(Audiences(p), "\x00") {
		return 0, ErrDenied
	}
	expires, err := claims.GetExpirationTime()
	if err != nil || expires == nil {
		return 0, ErrDenied
	}
	ttl := expires.Sub(now)
	if ttl <= 0 {
		return 0, ErrDenied
	}
	if ttl > 10*time.Minute {
		ttl = 10 * time.Minute
	}
	return ttl, nil
}

func Exchange(ctx context.Context, actor panelauth.Principal, namespace, name, bootstrap string) (string, int64, error) {
	if actor.TokenUse != panelauth.TokenUsePanel {
		return "", 0, ErrDenied
	}
	sdk := k8s.NewK8sClient().Sdk
	if err := authorizeActor(ctx, sdk, actor.Username, namespace); err != nil {
		return "", 0, err
	}
	obj, err := loadTarget(ctx, sdk.DynamicClient(), namespace, name)
	if err != nil {
		return "", 0, err
	}
	p := panelauth.Principal{Username: strings.TrimPrefix(namespace, "k3k-"), Actor: actor.Username, Role: "normal", PermissionName: "normal", TokenUse: panelauth.TokenUseCKMPanel, K3KNamespace: namespace, CVMName: name, CKMUID: string(obj.GetUID())}
	review, err := sdk.ClientSet.AuthenticationV1().TokenReviews().Create(ctx, &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: bootstrap}}, metav1.CreateOptions{})
	if err != nil || review == nil || !review.Status.Authenticated || review.Status.Error != "" {
		return "", 0, ErrDenied
	}
	ttl, err := validateBootstrap(bootstrap, review.Status.User.Username, sdk.GetNamespace(), p, time.Now())
	if err != nil {
		return "", 0, err
	}
	if actor.ExpiresAt > 0 && time.Until(time.Unix(actor.ExpiresAt, 0)) < ttl {
		ttl = time.Until(time.Unix(actor.ExpiresAt, 0))
	}
	token, err := panelauth.Issue(p, ttl)
	return token, time.Now().Add(ttl).Unix(), err
}

// Validate checks current authorization and object identity on every request.
func Validate(ctx context.Context, p panelauth.Principal) error {
	if p.TokenUse != panelauth.TokenUseCKMPanel || p.Role != "normal" || p.PermissionName != "normal" || p.ExpiresAt <= time.Now().Unix() {
		return ErrDenied
	}
	sdk := k8s.NewK8sClient().Sdk
	if err := authorizeActor(ctx, sdk, p.Actor, p.K3KNamespace); err != nil {
		return err
	}
	obj, err := loadTarget(ctx, sdk.DynamicClient(), p.K3KNamespace, p.CVMName)
	if err != nil || string(obj.GetUID()) != p.CKMUID {
		return ErrDenied
	}
	return nil
}

func Credential(ctx context.Context, p panelauth.Principal) (string, error) {
	if err := Validate(ctx, p); err != nil {
		return "", err
	}
	seconds := p.ExpiresAt - time.Now().Unix()
	if seconds <= 0 {
		return "", fmt.Errorf("%w: expired session", ErrDenied)
	}
	if seconds > 600 {
		seconds = 600
	}
	return k8s.NewK8sClient().Sdk.CreateTokenRequest("normal", seconds, Audiences(p))
}
