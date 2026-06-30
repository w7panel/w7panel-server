package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	corev1 "k8s.io/api/core/v1"
)

func Enabled() bool {
	return facade.Config.GetBool("logs.enabled")
}

func writer() Writer {
	return NewVictoriaLogsWriter()
}

func RecordLoginSuccess(ctx *gin.Context, username string, method string, sa *corev1.ServiceAccount) {
	if !Enabled() {
		return
	}
	user := userFromServiceAccount(sa)
	log := LoginLog{
		Time:        time.Now(),
		AuditType:   TypeLogin,
		Tenant:      user.Tenant,
		Username:    username,
		UserMode:    user.UserMode,
		LoginMethod: method,
		Success:     true,
		IP:          clientIP(ctx),
		UserAgent:   ctx.Request.UserAgent(),
		Message:     "login success",
	}
	go safeWriteLogin(log)
}

func RecordLoginSuccessUser(ctx *gin.Context, username string, method string, u *userservice.User) {
	if !Enabled() {
		return
	}
	user := userFromUser(u)
	log := LoginLog{
		Time:        time.Now(),
		AuditType:   TypeLogin,
		Tenant:      user.Tenant,
		Username:    username,
		UserMode:    user.UserMode,
		LoginMethod: method,
		Success:     true,
		IP:          clientIP(ctx),
		UserAgent:   ctx.Request.UserAgent(),
		Message:     "login success",
	}
	go safeWriteLogin(log)
}

func RecordLoginFailure(ctx *gin.Context, username string, method string, err error) {
	if !Enabled() {
		return
	}
	tenant := facade.Config.GetString("k8s.default_namespace")
	if tenant == "" {
		tenant = "default"
	}
	log := LoginLog{
		Time:        time.Now(),
		AuditType:   TypeLogin,
		Tenant:      tenant,
		Username:    username,
		LoginMethod: method,
		Success:     false,
		Reason:      sanitizeError(err),
		IP:          clientIP(ctx),
		UserAgent:   ctx.Request.UserAgent(),
		Message:     "login failed",
	}
	go safeWriteLogin(log)
}

func RecordOperation(ctx *gin.Context, start time.Time) {
	if !Enabled() {
		return
	}
	user := CurrentUser(ctx)
	if user.Username == "" {
		return
	}
	status := ctx.Writer.Status()
	log := OperationLog{
		Time:       start,
		AuditType:  TypeOperation,
		Tenant:     user.Tenant,
		Username:   user.Username,
		UserMode:   user.UserMode,
		Method:     ctx.Request.Method,
		Path:       ctx.Request.URL.Path,
		Route:      ctx.FullPath(),
		Params:     sanitizeParams(ctx.Params),
		StatusCode: status,
		Success:    status < 400,
		DurationMs: time.Since(start).Milliseconds(),
		IP:         clientIP(ctx),
		UserAgent:  ctx.Request.UserAgent(),
	}
	go safeWriteOperation(log)
}

func CurrentUser(ctx *gin.Context) UserContext {
	tokenStr := ctx.GetString("k8s_token")
	user := UserContext{
		Tenant:   facade.Config.GetString("k8s.default_namespace"),
		Username: ctx.GetString("username"),
		UserMode: "normal",
	}
	if mode := ctx.GetString("user_mode"); mode != "" {
		user.UserMode = mode
		user.IsAdmin = mode == "founder" || mode == "cluster"
	}
	if tokenStr == "" {
		return user
	}
	if ctx.GetString("panel_token") != "" {
		if user.Tenant == "" {
			user.Tenant = "default"
		}
		return user
	}
	token := k8s.NewK8sToken(tokenStr)
	if name, err := token.GetUserName(); err == nil && name != "" {
		user.Username = name
	}
	user.UserMode = token.GetRole()
	user.IsAdmin = user.UserMode == "founder" || user.UserMode == "cluster"
	if cfg, err := token.GetK3kConfig(); err == nil && cfg != nil {
		user.Tenant = cfg.Namespace
		user.K3kName = cfg.Name
		user.K3kNamespace = cfg.Namespace
	}
	if user.Tenant == "" {
		user.Tenant = "default"
	}
	return user
}

func userFromUser(u *userservice.User) UserContext {
	user := UserContext{
		Tenant:   "default",
		UserMode: "normal",
	}
	if u == nil {
		return user
	}
	user.Username = u.Name
	user.UserMode = u.Spec.UserMode
	if user.UserMode == "" {
		user.UserMode = u.Spec.Role
	}
	if user.UserMode == "" {
		user.UserMode = "normal"
	}
	user.IsAdmin = user.UserMode == "founder" || user.UserMode == "cluster"
	return user
}

func userFromServiceAccount(sa *corev1.ServiceAccount) UserContext {
	user := UserContext{
		Tenant:   "default",
		UserMode: "normal",
	}
	if sa == nil {
		return user
	}
	user.Username = sa.Name
	user.Tenant = sa.Namespace
	if sa.Labels != nil && sa.Labels[k3ktypes.W7_USER_MODE] != "" {
		user.UserMode = sa.Labels[k3ktypes.W7_USER_MODE]
	}
	user.IsAdmin = user.UserMode == "founder" || user.UserMode == "cluster"
	if sa.Annotations != nil {
		user.K3kName = sa.Annotations[k3ktypes.K3K_NAME]
		user.K3kNamespace = sa.Annotations[k3ktypes.K3K_NAMESPACE]
		if user.K3kNamespace != "" {
			user.Tenant = user.K3kNamespace
		}
	}
	return user
}

func safeWriteLogin(log LoginLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := writer().WriteLogin(ctx, log); err != nil {
		slog.Error("write login audit log failed", "err", err)
	}
}

func safeWriteOperation(log OperationLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := writer().WriteOperation(ctx, log); err != nil {
		slog.Error("write operation audit log failed", "err", err)
	}
}
