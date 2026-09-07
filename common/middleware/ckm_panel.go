package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/ckmsession"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/panelauth"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Child agents accept only the target cluster's own execution account. A host
// panel JWT or a merely decodable Kubernetes token never passes this boundary.
func processChildPanel(ctx *gin.Context) {
	raw := strings.TrimPrefix(ctx.GetHeader("Authorization"), "Bearer ")
	name := os.Getenv("K3K_NAME")
	if raw == "" || name == "" {
		ctx.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	sdk := k8s.NewK8sClient().Sdk
	review, err := sdk.ClientSet.AuthenticationV1().TokenReviews().Create(ctx.Request.Context(), &authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: raw}}, metav1.CreateOptions{})
	if err != nil || review == nil || !review.Status.Authenticated || review.Status.Error != "" || review.Status.User.Username != "system:serviceaccount:"+sdk.GetNamespace()+":"+name {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"msg": "目标集群凭据无效"})
		return
	}
	ctx.Set("k8s_token", raw)
	ctx.Set("user_mode", "normal")
	ctx.Next()
}

func ckmLocalMetadata(path string) bool {
	return path == "/panel-api/v1/auth/userinfo" || path == "/panel-api/v1/k3k/info" || path == "/panel-api/v1/auth/console/info"
}

func processCKMPanel(ctx *gin.Context, p panelauth.Principal) {
	// Never allow a delegated session to mint another session or host credential.
	if ctx.Request.URL.Path == "/panel-api/v1/auth/ckm-session" || ctx.Request.URL.Path == "/panel-api/v1/auth/k8s-credentials/token" || strings.HasPrefix(ctx.Request.URL.Path, "/panel-api/v1/oidc/") {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "子面板会话不允许此操作"})
		return
	}
	token, err := ckmsession.Credential(ctx.Request.Context(), p)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"msg": "CKM 会话已失效或无权访问"})
		return
	}
	ctx.Set("panel_principal", p)
	ctx.Set("username", p.Username)
	ctx.Set("actor", p.Actor)
	ctx.Set("ckm_namespace", p.K3KNamespace)
	ctx.Set("ckm_name", p.CVMName)
	ctx.Set("permission_name", "normal")
	ctx.Set("user_mode", "normal")
	ctx.Set("k8s_token", token)
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
