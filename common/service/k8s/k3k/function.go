package k3k

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	console2 "github.com/w7panel/w7panel/common/service/console"
	"github.com/w7panel/w7panel/common/service/k8s"

	"github.com/w7panel/w7panel/common/service/k8s/k3k/overselling"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func LoginByServiceAccount(client *k8s.Sdk, sa *v1.ServiceAccount, seconds int64, updateK3kUser bool, cvmName string) (string, bool, error) {
	k3kUser := types.NewK3kUser(sa)
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
	_, err := RefreshK3kUser(k3kUser, client, updateK3kUser)
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
	client, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	//设置最后登录时间
	_, err = controllerutil.CreateOrPatch(context.TODO(), client, user.ServiceAccount, func() error {
		user.SetLoginTime()
		return nil
	})
	if err != nil {
		return err
	}
	return err
}

func TokenToK3kUser(token string) (*types.K3kUser, error) {
	rootSdk := k8s.NewK8sClient().Sdk
	ktoken := k8s.NewK8sToken(token)
	saName, err := ktoken.GetSaName()
	if err != nil {
		return nil, err
	}
	sa, err := rootSdk.GetServiceAccount(rootSdk.GetNamespace(), saName)
	if err != nil {
		return nil, err
	}
	user := types.NewK3kUser(sa)
	if ktoken.IsK3kCluster() {
		user.SetCvmName(ktoken.GetCvmName())
	}
	return RefreshK3kUser(user, rootSdk, false)
}

func TokenToCvm(ctx context.Context, token, namespace, name string) (*cvmv1alpha1.Cvm, error) {
	k8sToken := k8s.NewK8sToken(token)
	rootSdk := k8s.NewK8sClient()
	user, err := TokenToK3kUser(token)
	if err != nil {
		return nil, err
	}
	if !user.IsFounder() {
		namespace = k8sToken.GetNamespace()
	}
	return GetCvm(ctx, rootSdk.Sdk, namespace, name)
}

// 登录时候刷新用户权限
func RefreshK3kUser(user *types.K3kUser, rootSdk *k8s.Sdk, update bool) (*types.K3kUser, error) {
	// user := types.NewK3kUser(sa)
	// oldSa := user.ServiceAccount.DeepCopy()
	w7configRepo := config.NewW7ConfigRepository(rootSdk)
	if !user.IsCustomPermission() {
		menuConfig, err := rootSdk.ClientSet.CoreV1().ConfigMaps(user.GetNamespace()).Get(rootSdk.Ctx, user.GetMenuName(), metav1.GetOptions{})
		if err != nil {
			slog.Error("GetMenuConfig error", "error", err)
		}
		if err == nil {
			user.ReplaceMenu(menuConfig)
		}
	}
	// if !user.IsCustomQuota() {
	// 	quotaConfig, err := rootSdk.ClientSet.CoreV1().ConfigMaps(user.GetNamespace()).Get(rootSdk.Ctx, user.GetQuotaName(), metav1.GetOptions{})
	// 	if err != nil {
	// 		slog.Error("GetQuotaConfig error", "error", err)
	// 	}
	// 	if err == nil {
	// 		user.ReplaceQuota(quotaConfig)
	// 	}
	// }
	if !user.IsCustomCost() {
		costConfig, err := rootSdk.ClientSet.CoreV1().ConfigMaps(user.GetNamespace()).Get(rootSdk.Ctx, user.GetCostName(), metav1.GetOptions{})
		if err != nil {
			slog.Error("GetCostConfig error", "error", err)
		}
		if err == nil {
			err := user.ReplaceCost(costConfig)
			if err != nil {
				slog.Error("ReplaceCost error", "error", err)
				return nil, err
			}
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
		_, err := rootSdk.ClientSet.CoreV1().ServiceAccounts(user.GetNamespace()).Update(rootSdk.Ctx, user.ServiceAccount, metav1.UpdateOptions{})
		if err != nil {
			slog.Error("user update error", "error", err)
			return nil, err
		}
	}
	return user, nil
}

func NeedRelogin(token *k8s.K8sToken) bool {
	saName, err := token.GetSaName()
	if err != nil {
		return false
	}
	// if token.GetLockVersion() != GetSaVersion(saName) || token.GetK3kPolicyVersion() != GetPolicyVersion(token.GetPolicyName()) {
	if token.GetLockVersion() != types.GetSaVersion(saName) {
		return true
	}
	return false
}

func getServiceAccountResource(sa *corev1.ServiceAccount) *overselling.Resource {
	user := k3ktypes.NewK3kUser(sa)
	lqr := user.GetLimitRange()
	if lqr != nil {
		return lqr.GetHardResource()
	}
	return overselling.EmptyResource()
}

func getCvmResource(cvm *cvmv1alpha1.Cvm) *overselling.Resource {
	resourceSpec := cvm.Status.EffectiveResource
	if resourceSpec == nil {
		resourceSpec = &cvmv1alpha1.CvmResource{}
	}

	return cvmResourceToOverSelling(resourceSpec)
}

func cvmResourceToOverSelling(resourceSpec *cvmv1alpha1.CvmResource) *overselling.Resource {

	return &overselling.Resource{
		CPU:       resource.MustParse(fmt.Sprintf("%dm", resourceSpec.CPU)),
		Memory:    resource.MustParse(fmt.Sprintf("%dGi", resourceSpec.Memory)),
		Storage:   resource.MustParse(fmt.Sprintf("%dGi", resourceSpec.Storage)),
		BandWidth: resource.MustParse(fmt.Sprintf("%dM", resourceSpec.Bandwidth)),
	}
}

func RefreshK3kPolicy(policy *v1alpha1.VirtualClusterPolicy, rootSdk *k8s.Sdk, update bool) error {
	if policy.Annotations == nil {
		return nil
	}
	costName, ok := policy.Annotations["w7.cc/cost-name"]
	if ok && costName != "" {
		costConfig, err := rootSdk.ClientSet.CoreV1().ConfigMaps(policy.Namespace).Get(rootSdk.Ctx, costName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		json, err := types.ConfigMapToCostString(costConfig)
		if err != nil {
			return err
		}
		policy.Annotations["w7.cc/cost"] = json
	}
	return nil
}

func TryCheckOverSellingResource(sdk *k8s.Sdk, cvm *cvmv1alpha1.Cvm) error {
	if !cvm.CanOverSellingCheck() {
		// 不需要资源检测
		return nil
	}
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return err
	}
	//资源验证有问题
	err = overselling.CanAddResourceCvm(cvmResourceToOverSelling(cvm.Spec.PendingPurchasedResource), getCvmResource)
	if err != nil {
		return err
	}
	_, err = controllerutil.CreateOrPatch(context.TODO(), sigClient, cvm, func() error {
		cvm.CheckSuccess()
		return nil
	})

	// TODO: 尝试添加资源
	return err
}

func GetCvm(ctx context.Context, sdk *k8s.Sdk, namespace, cvmName string) (*cvmv1alpha1.Cvm, error) {
	if cvmName == "" {
		return nil, fmt.Errorf("cvm不能为空")
	}
	sigClient, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	cvm := &cvmv1alpha1.Cvm{}
	if err := sigClient.Get(ctx, client.ObjectKey{Name: cvmName, Namespace: namespace}, cvm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, err
		}
		return nil, err
	}
	return cvm, nil
}

func SyncUserToCvm(ctx context.Context, user *types.K3kUser, sdk *k8s.Sdk) error {
	if !user.IsOldClusterUser() {
		return nil
	}
	if user.IsExpired() {
		return nil
	}
	user.SetOverMode(true)
	lr := user.GetLimitRange()
	if lr == nil {
		return nil
	}
	_, err := GetCvm(ctx, sdk, user.GetK3kNamespace(), user.GetName())
	if err != nil {
		if apierrors.IsNotFound(err) {
			rs := lr.GetHardBuyResource()

			cvm := &cvmv1alpha1.Cvm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      user.GetName(),
					Namespace: user.GetK3kNamespace(),
				},
				Spec: cvmv1alpha1.CvmSpec{
					PurchasedResource: &cvmv1alpha1.CvmResource{
						CPU:       rs.Cpu,
						Memory:    rs.Memory,
						Storage:   rs.Storage,
						Bandwidth: rs.Bandwidth,
					},
					StorageClassName: lr.StorageClass,
					ExpireTime:       user.Annotations[k3ktypes.K3K_EXPIRE_TIME],
				},
			}
			return sdk.Create(ctx, cvm)
		}
	}
	return nil
}
