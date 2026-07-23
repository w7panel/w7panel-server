package k3k

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"
	"github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"

	cvmv1alpha1 "github.com/w7panel/w7panel/common/service/k8s/ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LoginByUser(client *k8s.Sdk, crdUser *v1alpha1.User, seconds int64, updateK3kUser bool, cvmName string, realSa string) (string, error) {

	k3kUser := types.NewK3kUser(crdUser)

	k8s.NewK8sClient().Clear(crdUser.Name, cvmName)

	_, err := RefreshK3kUser(k3kUser, client, updateK3kUser)
	if err != nil {
		return "", err
	}

	token, err := client.CreateTokenRequest(realSa, seconds, k3kUser.GetTokenAud(cvmName))
	if err != nil {
		return "", err
	}
	// 标记最后登录时间 为了触发面板代理重建
	if k3kUser.IsFounder() {
		go console.RegisterLicenseSite(k3kUser.Name)
	}

	go SignLastLoginTime(client, k3kUser)
	//刷新控制台token
	go func() {
		err := console.RefreshCDTokenUseOpenid(k3kUser.Name)
		if err != nil {
			slog.Warn("刷新CDToken失败", "err", err)
		}
	}()

	return token, nil
}

func SignLastLoginTime(sdk *k8s.Sdk, user *types.K3kUser) error {
	user.SetLoginTime()
	user.SyncSpecFromRuntime()
	_, err := userservice.UpdateSpec(context.TODO(), sdk, user.Name, user.Spec)
	return err
}

func TokenToK3kUser(token string) (*types.K3kUser, error) {
	rootSdk := k8s.NewK8sClient().Sdk
	ktoken := k8s.NewK8sToken(token)
	userName, err := ktoken.GetUserName()
	if err != nil {
		return nil, err
	}
	userCRD, err := userservice.Get(rootSdk.Ctx, rootSdk, userName)
	if err != nil {
		return nil, err
	}
	user := types.NewK3kUser(userCRD.ToTyped())
	if ktoken.IsK3kCluster() {
		user.SetCkmName(ktoken.GetCvmName())
	}
	return RefreshK3kUser(user, rootSdk, false)
}

func TokenToCkm(ctx context.Context, token, namespace, name string) (*cvmv1alpha1.Ckm, error) {
	k8sToken := k8s.NewK8sToken(token)
	rootSdk := k8s.NewK8sClient()
	user, err := TokenToK3kUser(token)
	if err != nil {
		return nil, err
	}
	if !user.IsFounder() {
		namespace = k8sToken.GetNamespace()
	}
	return GetCkm(ctx, rootSdk.Sdk, namespace, name)
}

// 登录时候刷新用户权限
func RefreshK3kUser(user *types.K3kUser, rootSdk *k8s.Sdk, update bool) (*types.K3kUser, error) {
	w7configRepo := config.NewW7ConfigRepository(rootSdk)
	if user.GetPermissionName() == "" && user.IsFounder() {
		user.Spec.PermissionName = permissionservice.FounderPermissionName
	}
	if user.GetPermissionName() == "" && user.IsNormal() {
		user.Spec.PermissionName = permissionservice.NormalPermissionName
	}
	if !user.IsCustomPermission() {
		permissionConfig, err := permissionservice.Get(rootSdk.Ctx, rootSdk, user.GetPermissionName())
		if err == nil {
			permissionservice.EnsureBuiltinDefaults(permissionConfig)
			user.ApplyPermission(permissionConfig.Name, permissionConfig.Spec.Role, permissionservice.MenuRules(permissionConfig), permissionConfig.Spec.Features, permissionConfig.Spec.DomainWhiteList, permissionservice.APIRules(permissionConfig))
		}
		if err != nil {
			slog.Error("GetPermission error", "error", err)
		}
	}

	if user.IsCkmReqUser() {
		ckmName := user.GetCkmName()
		k3kNs := user.GetK3kNamespace()
		cvm, err := GetCkm(context.TODO(), rootSdk, k3kNs, ckmName)
		if err != nil {
			slog.Error("GetCkm error", "error", err)
		}
		if cvm != nil {

			user.ReplaceCkm(cvm) //
		}
	}

	w7config, err := w7configRepo.Get(user.Name)
	if err != nil {
		slog.Error("GetW7Config error", "error", err)
	}
	if w7config != nil {
		user.ReplaceW7Config(w7config)
	}
	// user.SetOverMode(true)
	if update {
		user.SyncSpecFromRuntime()
		_, err := userservice.UpdateSpec(rootSdk.Ctx, rootSdk, user.Name, user.Spec)
		if err != nil {
			slog.Error("user update error", "error", err)
			return nil, err
		}
	}
	return user, nil
}

func GetCkm(ctx context.Context, sdk *k8s.Sdk, namespace, cvmName string) (*cvmv1alpha1.Ckm, error) {
	if cvmName == "" {
		return nil, fmt.Errorf("cvm不能为空")
	}
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	cvm := &cvmv1alpha1.Ckm{}
	if err := sigClient.Get(ctx, client.ObjectKey{Name: cvmName, Namespace: namespace}, cvm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, err
	}
	return cvm, nil
}
