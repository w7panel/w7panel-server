package k3k

import (
	"context"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/console"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func PublishToShop(ctx context.Context, client client.Client, k3kpolicy *corev1.ConfigMap) error {
	if console.GetCurrentLicense() == nil {
		slog.Error("no license to publish")
		return nil
	}
	slog.Error("publish to shop")

	group := k3ktypes.NewK3kCostConfigMap(k3kpolicy)
	if !group.CanPublish() {
		// return nil
	}

	urlValues, err := group.ToPublishShopParams2(k3kpolicy.Name)
	if err != nil {
		return err
	}

	if config.MainW7Config == nil {
		slog.Error("no main config")
		return nil
	}
	city := config.CurrentCity
	if ncity, ok := k3kpolicy.Annotations["city"]; ok {
		city = ncity
	}
	// urlValues["description"] = k3kpolicy.Annotations["description"]
	urlValues["clusterid"] = config.MainW7Config.ClusterId
	urlValues["city"] = city
	urlValues["clusterurl"] = config.MainW7Config.OfflineUrl

	sdkClient, err := console.NewSdkClient(console.GetCurrentLicense())
	if err != nil {
		return err
	}
	return sdkClient.PublishPanelResource2(urlValues)

	// consolecdClient := console.NewConsoleCdClient(config.MainW7Config.ThirdpartyCDToken)
	// return consolecdClient.PublishPanelResource(urlValues)

}

func DeleteFromShop(k3kpolicy *corev1.ConfigMap) error {
	if console.GetCurrentLicense() == nil {
		slog.Error("no license")
		return nil
	}
	slog.Error("delete from shop")
	if config.MainW7Config == nil {
		slog.Error("no main config")
		return nil
	}
	// consolecdClient := console.NewConsoleCdClient(config.MainW7Config.ThirdpartyCDToken)
	data := map[string]string{
		"groupname": k3kpolicy.Name,
	}
	// return consolecdClient.DeletePanelResource(data)
	sdkClient, err := console.NewSdkClient(console.GetCurrentLicense())
	if err != nil {
		return err
	}
	return sdkClient.DeletePanelResource2(data)

}

func CheckPublish(ctx context.Context, r client.Client, k3kpolicy *corev1.ConfigMap) error {

	// cfg := &corev1.ConfigMap{}
	// if err := r.Get(ctx, types.NamespacedName{Namespace: "kube-system", Name: "k3k.config"}, cfg); err != nil {
	// 	if errors.IsNotFound(err) {
	// 		slog.Error("configmap not found")
	// 		return err
	// 	}
	// 	slog.Error("failed to get configmap")
	// 	return err
	// }
	// if cfg.Data["showInShop"] == "true" && k3kpolicy.Labels["w7.cc/showInShop"] == "true" {
	if k3kpolicy.Labels["w7.cc/showInShop"] == "true" {
		err := PublishToShop(ctx, r, k3kpolicy)
		if err != nil {
			slog.Error("failed to publish to shop ")
		}
		return nil
	}
	if k3kpolicy.Labels["w7.cc/showInShop"] != "true" {
		err := DeleteFromShop(k3kpolicy)
		if err != nil {
			slog.Error("failed to delete ")
		}
	}
	return nil
}
