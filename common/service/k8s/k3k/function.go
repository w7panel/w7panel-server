package k3k

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	console2 "github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	userservice "github.com/w7panel/w7panel/common/service/user"

	cvmv1alpha1 "github.com/w7panel/w7panel/common/service/k8s/ckm/api/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func LoginByServiceAccount(client *k8s.Sdk, sa *v1.ServiceAccount, seconds int64, updateK3kUser bool, cvmName string) (string, bool, error) {
	userCRD, err := userservice.Get(client.Ctx, client, sa.Name)
	if err != nil {
		return "", false, err
	}
	k3kUser := types.NewK3kUser(userCRD.ToTyped())
	isK3kUser := false
	// if k3kUser.IsClusterUser() {
	// 	isK3kUser = true
	// 	if sa.Annotations[types.K3K_CLUSTER_POLICY_VERSION] == "" {
	// 		sa.Annotations[types.K3K_CLUSTER_POLICY_VERSION] = "1"
	// 	}
	// 	policyName, ok := sa.Annotations[types.K3K_CLUSTER_POLICY]
	// 	if ok {
	// 		sa.Annotations[types.K3K_CLUSTER_POLICY_VERSION] = types.GetPolicyVersion(policyName)
	// 	}
	// }
	// if refreshCdToken {

	// }
	k8s.NewK8sClient().Clear(sa.Name, cvmName)
	_, err = RefreshK3kUser(k3kUser, client, updateK3kUser)
	if err != nil {
		return "", false, err
	}

	token, err := client.CreateTokenRequest(sa.Name, seconds, k3kUser.GetTokenAud(cvmName))
	if err != nil {
		return "", false, err
	}
	// 标记最后登录时间 为了触发面板代理重建
	if k3kUser.IsFounder() {
		go console.RegisterLicenseSite(k3kUser.Name)
		go func() {
			// 刷新license
			err = console2.VerifyDefaultLicense(true)
			if err != nil {
				slog.Error("刷新license失败", "err", err)
			}
		}()
	}

	go SignLastLoginTime(client, k3kUser)
	//刷新控制台token
	go func() {
		err := console.RefreshCDTokenUseOpenid(sa.Name)
		if err != nil {
			slog.Warn("刷新CDToken失败", "err", err)
		}
	}()

	return token, isK3kUser, nil
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
		user.SetCvmName(ktoken.GetCvmName())
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
	if user.GetMenuName() == "" && user.IsFounder() {
		user.Spec.PermissionName = permissionservice.FounderPermissionName
		user.Annotations[k3ktypes.W7_MENU_NAME] = permissionservice.FounderPermissionName
	}
	if user.GetMenuName() == "" && user.IsNormal() {
		user.Spec.PermissionName = permissionservice.NormalPermissionName
		user.Annotations[k3ktypes.W7_MENU_NAME] = permissionservice.NormalPermissionName
	}
	if !user.IsCustomPermission() {
		permissionConfig, err := permissionservice.Get(rootSdk.Ctx, rootSdk, user.GetMenuName())
		if err == nil {
			permissionservice.EnsureBuiltinDefaults(permissionConfig)
			user.ApplyPermission(permissionConfig.Name, permissionConfig.Spec.Role, permissionservice.MenuRules(permissionConfig), permissionConfig.Spec.Features, permissionConfig.Spec.DomainWhiteList, permissionservice.APIRules(permissionConfig))
		}
		if err != nil {
			slog.Error("GetPermission error", "error", err)
		}
	}

	if user.IsCvmReqUser() {
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
