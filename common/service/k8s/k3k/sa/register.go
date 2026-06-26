package sa

import (
	"context"
	"errors"
	"strconv"

	"github.com/w7corp/sdk-open-cloud-go/service"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	permissionservice "github.com/w7panel/w7panel/common/service/permission"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type Register struct {
	sdk          *k8s.Sdk
	client       client.Client
	k3kClient    *types.K3kClient
	w7configRepo config.W7ConfigRepositoryInterface
}

func DoRegister(sdk *k8s.Sdk, accessToken *types.ConsoleOAuthAccessToken, userinfo *service.ResultUserinfo, policyName string) (*corev1.ServiceAccount, error) {
	client, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	k3kClient := types.NewK3kClient(client)
	kconfig, err := k3kClient.GetK3kConfigSetting()
	if err != nil {
		return nil, err
	}
	// if policyName != "" {
	// 	register := NewRegister(client, sdk)
	// 	return register.RegisterUseConsole(accessToken, userinfo, kconfig, policyName)
	// }
	if kconfig.AllowConsoleRegister {
		register := NewRegister(client, sdk)
		return register.RegisterUseConsole(accessToken, userinfo, kconfig)
	} else {
		return nil, errors.New("不允许控制台注册")
	}
}

func DoRegisterByUid(sdk *k8s.Sdk, uid int) (*corev1.ServiceAccount, error) {
	client, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	k3kClient := types.NewK3kClient(client)
	kconfig, err := k3kClient.GetK3kConfigSetting()
	if err != nil {
		return nil, err
	}
	if kconfig.AllowConsoleRegister && kconfig.DefaultPermissionName != "" {
		register := NewRegister(client, sdk)
		return register.RegisterUid(uid, kconfig)
	} else {
		return nil, errors.New("不允许控制台注册")
	}
}

// DoRegisterLink 链接注册账号
func DoRegisterLink(sdk *k8s.Sdk, username, password, policyName string) (*corev1.ServiceAccount, error) {
	client, err := sdk.ToSigClient()
	if err != nil {
		return nil, err
	}
	sa, err := sdk.GetServiceAccount("default", username)
	if err != nil {
		//not found 忽略
		if !apierrors.IsNotFound(err) {
			return nil, errors.New("账号已存在..")
		}
	}
	if (err == nil) && sa != nil {
		return nil, errors.New("账号已存在.")
	}

	bpassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	register := NewRegister(client, sdk)
	annotations := map[string]string{
		"password": string(bpassword),
	}
	return register.doRegister(username, "", annotations, true, policyName)
}

func NewRegister(client client.Client, sdk *k8s.Sdk) *Register {
	respo := config.NewW7ConfigRepository(sdk)
	return &Register{
		client:       client,
		sdk:          sdk,
		k3kClient:    types.NewK3kClient(client),
		w7configRepo: respo,
	}
}

func (register *Register) RegisterUseConsole(accessToken *types.ConsoleOAuthAccessToken, userinfo *service.ResultUserinfo, k3kConfig *types.K3kConfigSetting) (*corev1.ServiceAccount, error) {
	userId := strconv.Itoa(userinfo.UserId)
	openId := userinfo.OpenId
	anns := map[string]string{
		types.W7_ACCESS_TOKEN:    accessToken.ToString(),
		"w7.cc/menu-name":        "k3k.permission.normal",
		"w7.cc/console-nickname": userinfo.Nickname,
		"w7.cc/console-openid":   openId,
	}
	return register.doRegister("console-"+userId, userId, anns, false, k3kConfig.DefaultPermissionName)
}

func (register *Register) RegisterUid(uid int, k3kConfig *types.K3kConfigSetting) (*corev1.ServiceAccount, error) {
	userId := strconv.Itoa(uid)
	return register.doRegister("console-"+userId, userId, nil, false, k3kConfig.DefaultPermissionName)
}

func (register *Register) doRegister(saName string, consoleId string, anns map[string]string, checkAllowRegister bool, permissionName string) (*corev1.ServiceAccount, error) {

	labels := map[string]string{
		"w7.cc/role":      "normal",
		"w7.cc/w7panel":   "true",
		"w7.cc/user-mode": "normal",
	}
	if consoleId != "0" && consoleId != "" {
		labels["w7.cc/console-id"] = consoleId
	}
	annotations := map[string]string{}

	if anns != nil {
		for k, v := range anns {
			annotations[k] = v
		}
	}
	if permissionName != "" {
		annotations[types.W7_MENU_NAME] = permissionName
	}

	sa := &corev1.ServiceAccount{

		ObjectMeta: metav1.ObjectMeta{
			Name:        saName,
			Namespace:   "default",
			Labels:      labels,
			Annotations: annotations,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
	}
	if permissionName != "" {
		permissionConfig, err := permissionservice.Get(context.Background(), register.sdk, permissionName)
		if err != nil {
			return nil, err
		}
		permissionservice.ApplyToServiceAccount(sa, permissionConfig)
	}
	// sa 已存在
	// sa, err = register.sdk.ClientSet.CoreV1().ServiceAccounts("default").Create(register.sdk.Ctx, sa, metav1.CreateOptions{})
	// if err != nil {
	// 	return nil, err
	// }
	_, err := controllerutil.CreateOrPatch(context.Background(), register.client, sa, func() error {
		return nil
	})
	return sa, err

}
