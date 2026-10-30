package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/ckmsession"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	"github.com/w7panel/w7panel/common/service/panelauth"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A CKM-target panel accepts only its own execution account. A host panel JWT
// or a merely decodable Kubernetes token never passes this boundary.
func processChildPanel(ctx *gin.Context) {
	raw := strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer ")
	name := os.Getenv("K3K_NAME")
	if raw == "" || name == "" {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	if !allowExclusiveAPI(ctx, sdk, "normal") {
		return
	}
	review, err := sdk.ClientSet.AuthenticationV1().TokenReviews().Create(ctx.Request.Context(), &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: raw}}, metav1.CreateOptions{})
	if err != nil || review == nil || !review.Status.Authenticated || review.Status.Error != "" || review.Status.User.Username != "system:serviceaccount:"+sdk.GetNamespace()+":"+name {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "目标集群凭据无效"})
		return
	}
	setK8sToken(ctx, raw)
	ctx.Set("user_mode", "normal")
	ctx.Next()
}

func ckmLocalMetadata(path string) bool {
	return path == "/panel-api/v1/auth/userinfo" || path == "/panel-api/v1/auth/console/info" || path == "/panel-api/v1/auth/main-settings"
}

func authorizeCKMPanelRequest(permission *configv1alpha1.Permission, req *http.Request) (bool, error) {
	if ckmLocalMetadata(req.URL.Path) {
		return true, nil
	}
	if strings.HasPrefix(req.URL.Path, "/k8s-proxy/") {
		return permissionservice.AuthorizeCKMK8sProxy(permission, req), nil
	}
	return permissionservice.AuthorizeRouteWithPermission(permission, req.Method, req.URL.Path)
}

func processCKMPanel(ctx *gin.Context, p panelauth.Principal) {
	if mainCostProxyMutation(ctx.Request) {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "子面板会话不允许管理权限费用套餐"})
		return
	}
	// Never allow a delegated session to mint another session or host credential.
	if ctx.Request.URL.Path == "/panel-api/v1/auth/ckm-session" || ctx.Request.URL.Path == "/panel-api/v1/auth/k8s-credentials/token" || strings.HasPrefix(ctx.Request.URL.Path, "/panel-api/v1/oidc/") {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "子面板会话不允许此操作"})
		return
	}
	if !allowExclusiveAPI(ctx, k8s.NewK8sClient().Sdk, "normal") {
		return
	}
	token, err := ckmsession.Credential(ctx.Request.Context(), p)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "CKM 会话已失效或无权访问"})
		return
	}
	source, name, err := ckmsession.PermissionBinding(ctx.Request.Context(), p)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "CKM 权限绑定无效"})
		return
	}
	if source != "" && source != "panel" {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "CKM 权限来源无效"})
		return
	}
	if source == "panel" {
		permission, err := permissionservice.Get(ctx.Request.Context(), k8s.NewK8sClient().Sdk, name)
		if err != nil || permission.Spec.Role != "normal" {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "主面板权限不可用"})
			return
		}
		allowed, err := authorizeCKMPanelRequest(permission, ctx.Request)
		if err != nil || !allowed {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "主面板权限不允许此操作"})
			return
		}
	}
	ctx.Set("panel_principal", p)
	ctx.Set("username", p.Username)
	ctx.Set("actor", p.Actor)
	ctx.Set("ckm_namespace", p.K3KNamespace)
	ctx.Set("ckm_name", p.CVMName)
	ctx.Set("permission_name", "normal")
	ctx.Set("user_mode", "normal")
	setK8sToken(ctx, token)
	if ckmLocalMetadata(ctx.Request.URL.Path) {
		ctx.Next()
		return
	}
	// Execute all remaining panel APIs on the selected agent, never on the host.
	ctx.Request.Header.Del("X-W7Panel-K8s-Token")
	ctx.Request.Header.Del("X-W7Panel-Token")
	ctx.Request.Header.Del("Cookie")
	if strings.Contains(ctx.GetHeader("Sec-WebSocket-Protocol"), "w7panel-bearer.") {
		ctx.Request.Header.Set("Sec-WebSocket-Protocol", "w7panel-ckm")
	}
	ctx.Request.Header.Set("Authorization", "Bearer "+token)
	Proxy{}.Process(ctx)
}
