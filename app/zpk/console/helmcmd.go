package console

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"
	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/dnsprofile"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
)

type HelmCmd struct {
	console2.Abstract
}

type helmOption struct {
	ingressDNSGrants    string
	autoSSLHosts        []string
	version             string
	releaseName         string
	repository          string
	chartName           string
	namespace           string
	set                 []string
	setJson             []string
	atomic              bool   // 原子安装
	zipUrl              string // 压缩包地址 应用市场用zip 存helm包
	chartApprovalDigest string
	annotations         map[string]string // 注解信息
	labels              map[string]string // 标签数据
}

var helmOp = helmOption{}

func (c HelmCmd) GetName() string {
	return "helmgo"
}

func (c HelmCmd) Configure(cmd *cobra.Command) {
	cmd.Flags().StringVar(&helmOp.version, "version", "", "version")
	cmd.Flags().StringVar(&helmOp.releaseName, "releaseName", "", "安装的名称")
	cmd.Flags().StringVar(&helmOp.repository, "repository", "", "helm仓库地址")
	cmd.Flags().StringVar(&helmOp.namespace, "namespace", "", "namespace")
	cmd.Flags().StringVar(&helmOp.chartName, "chartName", "", "目标路径")
	cmd.Flags().StringArrayVar(&helmOp.set, "set", []string{}, "set参数")
	cmd.Flags().StringArrayVar(&helmOp.setJson, "set-json", []string{}, "set-json参数")
	cmd.Flags().StringVar(&helmOp.zipUrl, "zipUrl", "", "zip包地址")
	cmd.Flags().StringVar(&helmOp.chartApprovalDigest, "chartApprovalDigest", "", "已确认的 CRD 摘要")
	cmd.Flags().BoolVar(&helmOp.atomic, "atomic", false, "原子安装")
	cmd.Flags().StringToStringVar(&helmOp.annotations, "anno", map[string]string{}, "annotations参数")
	cmd.Flags().StringToStringVar(&helmOp.labels, "labels", map[string]string{}, "labels参数")
	cmd.Flags().StringSliceVar(&helmOp.autoSSLHosts, "auto-ssl-hosts", nil, "启用自动证书的域名")
	cmd.Flags().StringVar(&helmOp.ingressDNSGrants, "ingress-dns-grants", "", "安装泛域名授权")
	cmd.MarkFlagRequired("releaseName")
}

func (c HelmCmd) Handle(cmd *cobra.Command, args []string) {
	slog.Info("helmgo start")
	time.Sleep(time.Second * 5) //appgroup未创建完成，延迟执行
	sdk := k8s.NewK8sClient().Sdk
	helmApi := helmservice.NewHelm(sdk)
	helmApi.InstallationLog(slog.Default())
	helmApi.IngressAutoSSLHosts(helmOp.autoSSLHosts)
	if helmOp.ingressDNSGrants != "" {
		var grants []dnsprofile.HelmGrant
		if err := json.Unmarshal([]byte(helmOp.ingressDNSGrants), &grants); err != nil {
			slog.Error("invalid ingress DNS grants")
			os.Exit(1)
			return
		}
		helmApi.IngressDNSGrants(grants)
	}
	chart, err := logic.LocateChartByHelmZpk(helmOp.repository, helmOp.chartName, helmOp.zipUrl, helmOp.version)
	if err != nil {
		slog.Error("locate chart error", "error", err)
		os.Exit(1)
		return
	}
	if chart.Metadata.Annotations == nil {
		chart.Metadata.Annotations = make(map[string]string)
	}
	if helmOp.annotations != nil && len(helmOp.annotations) > 0 {
		for k, v := range helmOp.annotations {
			chart.Metadata.Annotations[k] = v
		}
	}

	settings := cli.New()
	optValues := &values.Options{}
	for _, val := range helmOp.set {
		optValues.StringValues = append(optValues.StringValues, val)
	}
	for _, val := range helmOp.setJson {
		optValues.JSONValues = append(optValues.JSONValues, val)
	}
	slog.Info("abcdefg merge values", "vals", optValues.JSONValues)
	provider := getter.All(settings)
	vals, err := optValues.MergeValues(provider)
	if err != nil {
		slog.Error("merge values error", "err", err)
		os.Exit(1)
		return
	}
	if helmOp.chartApprovalDigest != "" {
		if err := helmservice.ProcessChartDependencies(chart, helmOp.set, helmOp.setJson); err != nil {
			slog.Error("process chart dependencies error", "err", err)
			os.Exit(1)
			return
		}
		if err := verifyCRDApproval(chart, helmOp.chartApprovalDigest); err != nil {
			slog.Error("CRD approval error", "error", err)
			os.Exit(1)
			return
		}
	}

	info, err := helmApi.Info(helmOp.releaseName, helmOp.namespace)
	helmApi.Atomic(helmOp.atomic)
	if info != nil {
		_, err := helmApi.Upgrade(sdk.Ctx, chart, vals, helmOp.releaseName, helmOp.namespace, helmOp.labels)
		if err != nil {
			slog.Error("upgrade error", "err", err)
			os.Exit(1)
			return
		}
	} else {
		_, err := helmApi.Install(sdk.Ctx, chart, vals, helmOp.releaseName, helmOp.namespace, helmOp.labels)
		if err != nil {
			slog.Error("install error", "err", err)
			os.Exit(1)
			return
		}
	}
	slog.Info("helmgo install success")

}

func verifyCRDApproval(chart *chart.Chart, digest string) error {
	if digest == "" {
		return nil
	}
	crds, err := helmservice.ChartCRDs(chart)
	if err != nil {
		return err
	}
	actual, err := helmservice.CRDDigest(crds)
	if err != nil {
		return err
	}
	if actual != digest {
		return errors.New("CRDs changed since confirmation; check the upgrade again")
	}
	return nil
}
