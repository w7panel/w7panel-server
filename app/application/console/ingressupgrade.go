package console

import (
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"golang.org/x/mod/semver"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type IngressUpgrade struct {
	console2.Abstract
}

func (c IngressUpgrade) GetName() string {
	return "ingress-add-group"
}

func (c IngressUpgrade) Configure(cmd *cobra.Command) {

}

func (c IngressUpgrade) GetDescription() string {
	return "升级ingress信息到新版"
}

func (c IngressUpgrade) Handle(cmd *cobra.Command, args []string) {
	deploymentName, ok := os.LookupEnv("DEPLOYMENT_NAME")
	if !ok {
		deploymentName = "w7panel"
	}
	slog.Info("开始升级ingress信息到新版")
	sdk := k8s.NewK8sClient().Sdk
	deadline := time.Now().Add(5 * time.Minute)
	for {
		if time.Now().After(deadline) {
			slog.Error("等待面板就绪超时", "deployment", deploymentName)
			os.Exit(1)
		}
		deployment, err := sdk.ClientSet.AppsV1().Deployments(sdk.GetNamespace()).Get(sdk.Ctx, deploymentName, metav1.GetOptions{})
		if err != nil {
			slog.Error("获取面板信息失败", slog.String("error", err.Error()))
			time.Sleep(3 * time.Second)
			continue
		}
		if c.IsReady(deployment) {
			slog.Info("面板 已就绪")
			break
		}
		time.Sleep(3 * time.Second)
	}
	old, err := appgroup.NewOldUpgrade(sdk)
	if err != nil {
		slog.Error("新版升级失败", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := old.Upgrade(); err != nil {
		slog.Error("升级 Ingress 应用分组失败", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func (c IngressUpgrade) IsReady(deployment *appsv1.Deployment) bool {
	if deployment == nil || deployment.Spec.Replicas == nil || len(deployment.Spec.Template.Spec.Containers) == 0 {
		return false
	}
	if deployment.Status.ReadyReplicas == *deployment.Spec.Replicas && deployment.Generation == deployment.Status.ObservedGeneration {
		envs := deployment.Spec.Template.Spec.Containers[0].Env
		for _, env := range envs {
			if env.Name == "HELM_VERSION" {
				version := env.Value
				slog.Info("current面板版本", slog.String("version", version))
				if semver.Compare("v"+version, "v1.0.39") >= 0 {
					slog.Info("面板版本大于1.0.39")
					return true
				}
			}
		}
		// return true
	}
	return false
}
