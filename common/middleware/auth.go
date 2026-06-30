package middleware

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/middleware"
	"gopkg.in/yaml.v3"
)

var (
	mockToken     string
	mockTokenOnce sync.Once
	mockTokenErr  error
)

type Auth struct {
	middleware.Abstract
}

func (self Auth) Process(ctx *gin.Context) {

	// LOCAL_MOCK 模式下绕过认证（用于本地开发测试）
	if os.Getenv("LOCAL_MOCK") == "true" || os.Getenv("LOCAL_MOCK") == "1" {
		// 尝试从 kubeconfig 读取有效 token
		token := getMockToken()
		ctx.Set("k8s_token", token)
		ctx.Next()
		return
	}

	bearertoken := self.getToken(ctx)
	if bearertoken == "" {
		ctx.AbortWithStatusJSON(401, gin.H{
			"code": 401,
			"msg":  "请登录",
		})
		return
	}
	if self.processPanelToken(ctx, bearertoken) {
		return
	}
	k8sToken := k8s.NewK8sToken(bearertoken)
	if k8sToken.IsCacheToken() {
		if saName, err := k8sToken.GetUserName(); err == nil {
			ctx.Set("username", saName)
			// TODO 兼容非 k3k 集群，非k3k 集群不校验权限
			if !self.authorizePanelAPI(ctx, saName) {
				return
			}
		}
		ctx.Set("k8s_token", bearertoken)
		ctx.Next()
		return
	}

	err := k8s.NewK8sClient().TokenReview(bearertoken)
	if err != nil {
		ctx.AbortWithStatusJSON(401, gin.H{
			"code": 401,
			"msg":  "请登录" + err.Error(),
		})
		return
	}
	k8sToken.Cache()

	userName, err := k8sToken.GetUserName()
	if err == nil {
		ctx.Set("username", userName)
		if !self.authorizePanelAPI(ctx, userName) {
			return
		}
	}
	ctx.Set("k8s_token", bearertoken)
	// if facade.Config.GetBool("app.refresh_token_enable") {
	// 	if ctx.Writer.Status() >= http.StatusOK && ctx.Writer.Status() < 300 {
	// 		saName, date := k8s.GetTokenSaName(bearertoken)
	// 		if saName != "" && !date.IsZero() && date.After(time.Now().Add(-time.Minute*10)) {
	// 			token, err := k8s.NewK8sClient().CreateTokenRequest(saName, facade.Config.GetInt64("app.login_seconds"), []string{})
	// 			if err != nil {
	// 				slog.Info("refresh token err: ", "err", err)
	// 				return
	// 			}

	// 			ctx.Writer.Header().Set("access-token", token)
	// 		}
	// 	}
	// }
	ctx.Next()

	// ctx.Writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
}

func (self Auth) processPanelToken(ctx *gin.Context, bearertoken string) bool {
	claims, err := userservice.ParseToken(bearertoken)
	if err != nil {
		return false
	}
	sdk := k8s.NewK8sClient().Sdk
	u, err := userservice.Get(ctx.Request.Context(), sdk, claims.Username)
	if err != nil {
		ctx.AbortWithStatusJSON(401, gin.H{"code": 401, "msg": "请登录"})
		return true
	}
	p, err := userservice.ResolvePermission(ctx.Request.Context(), sdk, u)
	if err != nil {
		ctx.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "没有权限: " + err.Error()})
		return true
	}
	allowed, err := permissionservice.AuthorizePanelAPIWithPermission(ctx.Request.Context(), sdk, p, ctx.Request.Method, ctx.Request.URL.Path)
	if err != nil {
		ctx.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "没有权限: " + err.Error()})
		return true
	}
	if !allowed {
		ctx.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "没有权限"})
		return true
	}
	execSA, err := userservice.ExecutionServiceAccount(ctx.Request.Context(), sdk, u)
	if err != nil {
		ctx.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "没有权限: " + err.Error()})
		return true
	}
	role := u.Spec.Role
	if role == "" {
		role = u.Spec.UserMode
	}
	audiences := []string{u.Name, role, u.Spec.ConsoleId, "", execSA, "https://kubernetes.default.svc.cluster.local", "k3s"}
	execToken, err := sdk.CreateTokenRequest(execSA, facade.Config.GetInt64("app.login_seconds"), audiences)
	if err != nil {
		ctx.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": fmt.Sprintf("创建执行token失败: %v", err)})
		return true
	}
	ctx.Set("username", u.Name)
	ctx.Set("user_mode", role)
	ctx.Set("permission_name", u.Spec.PermissionName)
	ctx.Set("panel_token", bearertoken)
	ctx.Set("k8s_token", execToken)
	ctx.Next()
	return true
}

func (self Auth) authorizePanelAPI(ctx *gin.Context, saName string) bool {
	allowed, err := permissionservice.AuthorizePanelAPI(ctx.Request.Context(), k8s.NewK8sClient().Sdk, saName, ctx.Request.Method, ctx.Request.URL.Path)
	if err != nil {
		ctx.AbortWithStatusJSON(403, gin.H{
			"code": 403,
			"msg":  "没有权限: " + err.Error(),
		})
		return false
	}
	if !allowed {
		ctx.AbortWithStatusJSON(403, gin.H{
			"code": 403,
			"msg":  "没有权限",
		})
		return false
	}
	return true
}

func (self Auth) getToken(ctx *gin.Context) string {
	return helper.GetToken(ctx)
}

// getMockToken 从 kubeconfig 读取 token（仅用于 LOCAL_MOCK 模式）
// 使用 sync.Once 缓存结果，避免每次请求都读文件
func getMockToken() string {
	mockTokenOnce.Do(func() {
		kubeconfigPaths := []string{
			"./kubeconfig.yaml",
			"./config/kubeconfig.yaml",
			"./w7panel/kubeconfig.yaml",
		}

		// 优先使用环境变量指定的路径
		if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
			kubeconfigPaths = append([]string{envPath}, kubeconfigPaths...)
		}

		// 尝试基于 KO_DATA_PATH 推断路径
		if koDataPath := os.Getenv("KO_DATA_PATH"); koDataPath != "" {
			kubeconfigPaths = append(kubeconfigPaths, filepath.Join(filepath.Dir(koDataPath), "kubeconfig.yaml"))
		}

		for _, path := range kubeconfigPaths {
			if token := readTokenFromKubeconfig(path); token != "" {
				mockToken = token
				return
			}
		}

		mockToken = "local-mock-token"
	})
	return mockToken
}

// readTokenFromKubeconfig 从 kubeconfig 文件读取 token
func readTokenFromKubeconfig(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var config struct {
		Users []struct {
			User struct {
				Token string `yaml:"token"`
			} `yaml:"user"`
		} `yaml:"users"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return ""
	}

	for _, u := range config.Users {
		if u.User.Token != "" {
			return u.User.Token
		}
	}

	return ""
}
