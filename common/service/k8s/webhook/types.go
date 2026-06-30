package webhook

import (
	"os"

	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var svcName = ""

type WebhookConfig struct {
	Name        string
	Description string
	URL         string
	Secret      string
}

func init() {
	name, ok := os.LookupEnv("SVC_NAME")
	if !ok {
		// slog.Error("SVC_NAME not set")
		return
	}
	svcName = name
}
func getSvcHost(namespace string) string {
	return svcName + "." + namespace + ".svc"
}

func getSecret() string {
	return svcName + "-webhook-tls"
}

func getHookName() string {
	return svcName + "-webhook"
}

func getHookCrdName() string {
	return svcName + "-crd-webhook"
}

func getSa(client client.Client, sdk *k8s.Sdk, saName string) (*v1.ServiceAccount, error) {
	sa := &v1.ServiceAccount{}
	err := client.Get(sdk.Ctx, types.NamespacedName{Name: saName, Namespace: "default"}, sa) //获取不到 未找到原因
	if err != nil {
		if errors.IsNotFound(err) {
			sa2, err := sdk.ClientSet.CoreV1().ServiceAccounts("default").Get(sdk.Ctx, saName, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			return sa2, nil
		}
		return nil, err
	}
	return sa, err
}
