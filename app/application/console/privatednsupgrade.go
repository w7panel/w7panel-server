package console

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/w7panel/w7panel/common/service/coredns"
	"github.com/w7panel/w7panel/common/service/k8s"
	privatednsv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/privatedns/v1alpha1"
	console2 "github.com/we7coreteam/w7-rangine-go/v2/src/console"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type PrivateDNSUpgrade struct {
	console2.Abstract
}

type privateDNSUpgradeOption struct {
	overwrite bool
}

var privateDNSUpgradeOp = privateDNSUpgradeOption{}

var privateDNSResourceNameRe = regexp.MustCompile(`[^a-z0-9.-]+`)

func (c PrivateDNSUpgrade) GetName() string {
	return "privatedns-upgrade"
}

func (c PrivateDNSUpgrade) Configure(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&privateDNSUpgradeOp.overwrite, "overwrite", false, "覆盖已存在的 PrivateDNS CRD spec.records")
}

func (c PrivateDNSUpgrade) GetDescription() string {
	return "升级旧 CoreDNS 私有解析配置到 PrivateDNS CRD"
}

func (c PrivateDNSUpgrade) Handle(cmd *cobra.Command, args []string) {
	ctx := context.Background()
	sdk := k8s.NewK8sClient().Sdk
	count, err := MigrateCoreDNSCustomToPrivateDNS(ctx, sdk, privateDNSUpgradeOp.overwrite)
	if err != nil {
		slog.Error("升级 PrivateDNS 失败", "error", err)
		return
	}
	slog.Info("升级 PrivateDNS 完成", "count", count, "overwrite", privateDNSUpgradeOp.overwrite)
}

func MigrateCoreDNSCustomToPrivateDNS(ctx context.Context, sdk *k8s.Sdk, overwrite bool) (int, error) {
	if sdk == nil {
		return 0, fmt.Errorf("k8s sdk is nil")
	}
	if _, err := sdk.ClientSet.CoreV1().ConfigMaps(coredns.CoreDNSNamespace).Get(ctx, coredns.CoreDNSCustomName, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			slog.Info("旧 CoreDNS 私有解析配置不存在，跳过 PrivateDNS 升级",
				"namespace", coredns.CoreDNSNamespace,
				"configmap", coredns.CoreDNSCustomName,
			)
			return 0, nil
		}
		return 0, err
	}

	service := coredns.NewServiceWithSdk(sdk)
	zones, err := service.ListZones(ctx)
	if err != nil {
		return 0, err
	}
	existing, err := listPrivateDNSByDomain(ctx, sdk)
	if err != nil {
		return 0, err
	}

	migrated := 0
	for _, zone := range zones {
		domain, err := coredns.NormalizeDomain(zone.Domain)
		if err != nil {
			slog.Warn("跳过无效 PrivateDNS 域名", "domain", zone.Domain, "error", err)
			continue
		}
		records, err := service.ListRecords(ctx, domain)
		if err != nil {
			slog.Warn("读取旧 PrivateDNS 记录失败", "domain", domain, "error", err)
			continue
		}
		if current, ok := existing[domain]; ok {
			if !overwrite {
				slog.Info("PrivateDNS CRD 已存在，跳过", "domain", domain, "name", current.GetName())
				continue
			}
			next := current.DeepCopy()
			setPrivateDNSSpec(next, domain, records)
			if _, err := privateDNSResource(sdk).Update(ctx, next, metav1.UpdateOptions{}); err != nil {
				return migrated, err
			}
			migrated++
			slog.Info("更新 PrivateDNS CRD", "domain", domain, "name", next.GetName(), "records", len(records))
			continue
		}

		obj := newPrivateDNSObject(domain, records)
		if _, err := privateDNSResource(sdk).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				slog.Info("PrivateDNS CRD 已存在，跳过", "domain", domain, "name", obj.GetName())
				continue
			}
			return migrated, err
		}
		existing[domain] = obj
		migrated++
		slog.Info("创建 PrivateDNS CRD", "domain", domain, "name", obj.GetName(), "records", len(records))
	}

	return migrated, nil
}

func privateDNSResource(sdk *k8s.Sdk) dynamic.NamespaceableResourceInterface {
	return sdk.DynamicClient().Resource(schema.GroupVersionResource{
		Group:    privatednsv1alpha1.SchemeGroupVersion.Group,
		Version:  privatednsv1alpha1.SchemeGroupVersion.Version,
		Resource: "privatedns",
	})
}

func listPrivateDNSByDomain(ctx context.Context, sdk *k8s.Sdk) (map[string]*unstructured.Unstructured, error) {
	list, err := privateDNSResource(sdk).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]*unstructured.Unstructured{}, nil
		}
		return nil, err
	}
	items := map[string]*unstructured.Unstructured{}
	for i := range list.Items {
		item := list.Items[i].DeepCopy()
		domain, _, _ := unstructured.NestedString(item.Object, "spec", "domain")
		domain, err := coredns.NormalizeDomain(domain)
		if err != nil {
			continue
		}
		items[domain] = item
	}
	return items, nil
}

func newPrivateDNSObject(domain string, records []coredns.Record) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(privatednsv1alpha1.SchemeGroupVersion.String())
	obj.SetKind("PrivateDNS")
	obj.SetName(privateDNSResourceName(domain))
	obj.SetLabels(map[string]string{
		"w7.cc/migrated-from": "coredns-custom",
	})
	setPrivateDNSSpec(obj, domain, records)
	return obj
}

func setPrivateDNSSpec(obj *unstructured.Unstructured, domain string, records []coredns.Record) {
	recordItems := make([]interface{}, 0, len(records))
	for _, record := range records {
		item := map[string]interface{}{
			"id":    record.ID,
			"name":  record.Name,
			"type":  record.Type,
			"value": record.Value,
			"ttl":   int64(record.TTL),
		}
		if record.Type == "MX" {
			item["mxPriority"] = int64(record.MXPriority)
		}
		recordItems = append(recordItems, item)
	}
	_ = unstructured.SetNestedField(obj.Object, domain, "spec", "domain")
	_ = unstructured.SetNestedSlice(obj.Object, recordItems, "spec", "records")
}

func privateDNSResourceName(domain string) string {
	name := strings.Trim(privateDNSResourceNameRe.ReplaceAllString(strings.ToLower(domain), "-"), "-")
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(name, "-"), "-")
	if name == "" {
		return "privatedns"
	}
	if len(name) > 253 {
		return strings.Trim(name[:253], "-")
	}
	return name
}
