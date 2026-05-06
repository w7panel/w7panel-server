package microapp

import (
	"errors"
	"log/slog"

	"github.com/samber/lo"
	"github.com/w7panel/w7panel/common/service/k8s"
	microapp "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	sig "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// 头部显示
func ListTop(t string) (*microapp.MicroAppList, error) {
	token := k8s.NewK8sToken(t)
	role := token.GetRole()
	if role == "" {
		return nil, errors.New("role is empty")
	}
	rootSdk := k8s.NewK8sClient().Sdk
	clientSdk, err := k8s.NewK8sClient().Channel(t)
	if err != nil {
		return nil, err
	}
	newList := &microapp.MicroAppList{}
	currentList, err := loadMicroAppList(clientSdk)
	if err != nil {
		return nil, err
	}
	rList := &microapp.MicroAppList{}
	if token.IsK3kCluster() {
		rootList, err := loadMicroAppList(rootSdk)
		if err != nil {
			return nil, err
		}
		rList = rootList
	}
	rList.Items = lo.Map(rList.Items, func(item microapp.MicroApp, index int) microapp.MicroApp {
		filterMicroapp(&item, role)
		return item
	})
	newList.Items = append(rList.Items, currentList.Items...)
	return newList, nil
}

func ListInfo(t string, name string) (*microapp.MicroApp, error) {
	token := k8s.NewK8sToken(t)
	role := token.GetRole()
	if role == "" {
		return nil, errors.New("role is empty")
	}
	rootSdk := k8s.NewK8sClient().Sdk
	currentRole := token.GetRole()
	clientSdk, err := k8s.NewK8sClient().Channel(t)
	if err != nil {
		return nil, err
	}
	microapp, err := loadMicroApp(clientSdk, name)
	if err != nil {
		rootMicroapp, err := loadMicroApp(rootSdk, name)
		if err != nil {
			return nil, err
		}
		filterMicroapp(rootMicroapp, currentRole)
		return rootMicroapp, nil
	}
	return microapp, nil
}
func filterMicroapp(item *microapp.MicroApp, role string) {
	if item.Labels == nil {
		item.Labels = map[string]string{}
	}
	item.Labels["microapp.w7.cc/from"] = "root"
	item.Spec.Bindings = lo.Filter(item.Spec.Bindings, func(bindings microapp.Bindings, index int) bool {
		return bindings.Name == role
	})
	newRole := item.Spec.ConfigV2.Props.RoleConfig[role]
	item.Spec.ConfigV2.Props.RoleConfig = map[string]microapp.Role{}
	item.Spec.ConfigV2.Props.RoleConfig[role] = newRole
}
func loadMicroApp(sdk *k8s.Sdk, name string) (*microapp.MicroApp, error) {
	microapp := &microapp.MicroApp{}
	sigClient, err := sdk.ToSigClient()
	if err != nil {

		return nil, err
	}
	err = sigClient.Get(sdk.Ctx, types.NamespacedName{Name: name, Namespace: "default"}, microapp)
	if err != nil {
		slog.Error("loadMicroApp", "err", err)
		return nil, err
	}
	return microapp, nil
}
func loadMicroAppList(sdk *k8s.Sdk) (*microapp.MicroAppList, error) {
	list := &microapp.MicroAppList{}
	sigClient, err := sdk.ToSigClient()
	if err != nil {

		return nil, err
	}
	err = sigClient.List(sdk.Ctx, list, &sig.ListOptions{})
	if err != nil {
		slog.Error("loadMicroAppList", "err", err)
		return nil, err
	}
	return list, nil
}

func patchRootMicroApp(sdk *k8s.Sdk, origin *microapp.MicroApp, role string) error {
	sigclient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("createMicroApp", "err", err)
		return err
	}
	item := origin.DeepCopy()
	item.Name = item.Name + "-root" //防止同名
	// itemCopy := item.DeepCopy()
	_, err = controllerutil.CreateOrPatch(sdk.Ctx, sigclient, item, func() error {
		item.Labels["microapp.w7.cc/from"] = "root"
		item.SetResourceVersion("")
		item.SetUID("")
		// 移除不属于当前角色的权限配置信息
		item.Spec.Bindings = origin.Spec.Bindings
		item.Spec.Bindings = lo.Filter(item.Spec.Bindings, func(bindings microapp.Bindings, index int) bool {
			return bindings.Name == role
		})
		newRole := item.Spec.ConfigV2.Props.RoleConfig[role]
		item.Spec.ConfigV2.Props.RoleConfig = map[string]microapp.Role{}
		item.Spec.ConfigV2.Props.RoleConfig[role] = newRole
		return nil
	})
	return err
}

func delMicroApp(sdk *k8s.Sdk, item *microapp.MicroApp) error {
	sigclient, err := sdk.ToSigClient()
	if err != nil {
		slog.Error("createMicroApp", "err", err)
		return err
	}
	return sigclient.Delete(sdk.Ctx, item)
}
