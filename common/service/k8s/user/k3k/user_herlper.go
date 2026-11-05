package k3k

import (
	"log/slog"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/k8s/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"

	"github.com/w7panel/w7panel/common/service/k8s/user/k3k/types"
)

func SignLastLoginTime(sdk *k8s.Sdk, user *types.K3kUser) error {
	user.SetLoginTime()
	return userservice.UpdateLoginTime(context.TODO(), sdk, user.Name, user.Spec.LoginTime)
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
	return RefreshK3kUser(user, rootSdk, false)
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
		pName := user.GetPermissionName()
		permissionConfig, err := permissionservice.Get(rootSdk.Ctx, rootSdk, pName)
		if err == nil {
			permissionservice.EnsureBuiltinDefaults(permissionConfig)
			user.ApplyPermission(permissionConfig.Name, permissionConfig.Spec.Role, permissionservice.MenuRules(permissionConfig), permissionConfig.Spec.Features, permissionConfig.Spec.DomainWhiteList, permissionservice.APIRules(permissionConfig))
		}
		if err != nil {
			slog.Error("GetPermission error", "error", err)
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
