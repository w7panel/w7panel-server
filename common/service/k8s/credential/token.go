package credential

import (
	"context"
	"fmt"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"
)

// IssueForPrincipal creates a short-lived token on demand. It intentionally
// does not persist child-cluster credentials in the host cluster.
// func IssueForPrincipal(ctx context.Context, username, permissionName string, ttl time.Duration) (string, int64, error) {
// 	return IssueForPrincipalWithAudiences(ctx, username, permissionName, ttl, nil)
// }

// // IssueForPrincipalFromToken is kept for callers that provide an existing
// // credential, but credentials no longer carry a child-cluster audience.
// func IssueForPrincipalFromToken(ctx context.Context, username, permissionName, sourceToken string, ttl time.Duration) (string, int64, error) {
// 	return IssueForPrincipal(ctx, username, permissionName, ttl)
// }

// IssueForPrincipalWithAudiences creates a short-lived ServiceAccount token
// with the supplied Kubernetes audiences. CKM/K3K callers must provide the
// complete audience tuple required by the target virtual cluster; passing nil
// preserves Kubernetes' default audience selection.
func IssueForPrincipalWithAudiences(ctx context.Context, username, permissionName string, ttl time.Duration, audiences []string) (string, int64, error) {
	if username == "" {
		return "", 0, fmt.Errorf("username is required")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	sdk := k8s.NewK8sClient().Sdk
	user, err := userservice.Get(ctx, sdk, username)
	if err != nil {
		return "", 0, err
	}
	saName := permissionservice.NormalizePermissionName(permissionName)
	if saName == "" {
		var errResolve error
		saName, errResolve = userservice.ExecutionServiceAccount(ctx, sdk, user)
		if errResolve != nil {
			return "", 0, errResolve
		}
	}
	seconds := int64(ttl.Seconds())
	token, err := sdk.CreateTokenRequest(saName, seconds, audiences)
	if err != nil {
		return "", 0, err
	}
	return token, time.Now().Add(ttl).Unix(), nil
}
