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
func IssueForPrincipal(ctx context.Context, username, permissionName string, ttl time.Duration) (string, int64, error) {
	if username == "" {
		return "", 0, fmt.Errorf("username is required")
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	sdk := k8s.NewK8sClient().Sdk
	saName := permissionservice.NormalizePermissionName(permissionName)
	if saName == "" {
		user, err := userservice.Get(ctx, sdk, username)
		if err != nil {
			return "", 0, err
		}
		var errResolve error
		saName, errResolve = userservice.ExecutionServiceAccount(ctx, sdk, user)
		if errResolve != nil {
			return "", 0, errResolve
		}
	}
	seconds := int64(ttl.Seconds())
	token, err := sdk.CreateTokenRequest(saName, seconds, []string{})
	if err != nil {
		return "", 0, err
	}
	return token, time.Now().Add(ttl).Unix(), nil
}
