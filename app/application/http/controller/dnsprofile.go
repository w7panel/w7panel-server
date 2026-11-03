package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/dnsprofile"
	"github.com/we7coreteam/w7-rangine-go/v2/src/http/controller"
	"k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DNSProfile struct { controller.Abstract }

func (DNSProfile) canWriteIngress(ctx *gin.Context, namespace string) bool {
	if namespace == "" { ctx.JSON(http.StatusBadRequest, gin.H{"msg": "缺少命名空间"}); return false }
	token, ok := ctx.Get("k8s_token")
	credential, ok := token.(string)
	if !ok || credential == "" { ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法验证域名管理权限"}); return false }
	sdk, err := k8s.NewK8sClient().Channel(credential)
	if err != nil { ctx.JSON(http.StatusForbidden, gin.H{"msg": "无法验证域名管理权限"}); return false }
	for _, verb := range []string{"create", "update"} {
		review, err := sdk.ClientSet.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx.Request.Context(), &v1.SelfSubjectAccessReview{
			Spec: v1.SelfSubjectAccessReviewSpec{ResourceAttributes: &v1.ResourceAttributes{
				Namespace: namespace, Verb: verb, Group: "networking.k8s.io", Resource: "ingresses",
			}},
		}, metav1.CreateOptions{})
		if err == nil && review.Status.Allowed { return true }
	}
	ctx.JSON(http.StatusForbidden, gin.H{"msg": "没有该命名空间的域名写入权限"})
	return false
}

func (DNSProfile) store(ctx *gin.Context) (dnsprofile.Store, bool) {
	if ctx.GetString("username") == "" {
		ctx.JSON(http.StatusUnauthorized, gin.H{"msg": "请登录"})
		return dnsprofile.Store{}, false
	}
	return dnsprofile.Store{SDK: k8s.NewK8sClient().Sdk}, true
}

func (self DNSProfile) List(ctx *gin.Context) {
	store, ok := self.store(ctx); if !ok { return }
	profiles, err := store.List(ctx.Request.Context(), ctx.GetString("username"))
	if err != nil { self.JsonResponseWithServerError(ctx, err); return }
	self.JsonResponseWithoutError(ctx, profiles)
}

func (self DNSProfile) Create(ctx *gin.Context) {
	store, ok := self.store(ctx); if !ok { return }
	var request struct {
		Namespace string `json:"namespace" binding:"required"`
		Name string `json:"name" binding:"required"`
		Provider string `json:"provider" binding:"required"`
		Zone string `json:"zone" binding:"required"`
		Credentials map[string]string `json:"credentials" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil { ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()}); return }
	if !self.canWriteIngress(ctx, request.Namespace) { return }
	profile, err := store.Create(ctx.Request.Context(), ctx.GetString("username"), request.Name, request.Provider, request.Zone, request.Credentials)
	if err != nil { ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()}); return }
	self.JsonResponseWithoutError(ctx, profile)
}

func (self DNSProfile) Delete(ctx *gin.Context) {
	store, ok := self.store(ctx); if !ok { return }
	if err := store.Delete(ctx.Request.Context(), ctx.GetString("username"), ctx.Param("id")); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()}); return
	}
	self.JsonResponseWithoutError(ctx, gin.H{"deleted": true})
}

func (self DNSProfile) Grant(ctx *gin.Context) {
	store, ok := self.store(ctx); if !ok { return }
	var request struct {
		Namespace string `json:"namespace" binding:"required"`
		Name string `json:"name" binding:"required"`
		Host string `json:"host" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil { ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()}); return }
	if !self.canWriteIngress(ctx, request.Namespace) { return }
	annotations, err := store.Grant(ctx.Request.Context(), ctx.GetString("username"), ctx.Param("id"), request.Namespace, request.Name, request.Host)
	if err != nil { ctx.JSON(http.StatusBadRequest, gin.H{"msg": err.Error()}); return }
	self.JsonResponseWithoutError(ctx, annotations)
}
