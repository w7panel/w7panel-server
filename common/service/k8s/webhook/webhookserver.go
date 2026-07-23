package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/we7coreteam/w7-rangine-go/v2/pkg/support/facade"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// 全局缓存，用于存储 VirtualClusterPolicy 的信息
var (
	policyCache     = make(map[string]map[string]string) // 存储 policy name -> annotations
	policyCacheLock sync.RWMutex                         // 用于保护 policyCache 的读写锁
)

func Prepare(sdk *k8s.Sdk) error {
	mutete := NewWebHookMutate(sdk)
	namespace := sdk.GetNamespace()

	if facade.Config.GetBool("webhook.certManager.enabled") {
		injectCAFrom := fmt.Sprintf("%s/%s", namespace, getCertificateName())
		if err := mutete.CreateOrUpdateWithCertManager(injectCAFrom, svcName, namespace, getHookName(), getOperations()); err != nil {
			slog.Error("create or update webhook failed")
			return err
		}
		if err := mutete.CreateOrUpdateWithCertManager(injectCAFrom, svcName, namespace, getHookCrdName(), getCrdOperations()); err != nil {
			slog.Error("create or update webhook failed")
			return err
		}
		return nil
	}

	if err := ensureCertificates(namespace); err != nil {
		return err
	}
	caBound, err := os.ReadFile("/tmp/k8s-webhook-server/serving-certs/tls.crt")
	if err != nil {
		return err
	}

	if err := mutete.CreateOrUpdate(caBound, svcName, namespace, getHookName(), getOperations()); err != nil {
		slog.Error("create or update webhook failed")
		return err
	}
	if err := mutete.CreateOrUpdate(caBound, svcName, namespace, getHookCrdName(), getCrdOperations()); err != nil {
		slog.Error("create or update webhook failed")
		return err
	}
	return nil
}

// ResourceMutator 处理各种资源的 webhook
type ResourceMutator struct {
	decoder admission.Decoder
	client  client.Client
	sdk     *k8s.Sdk
}

func NewResourceMutator(client client.Client, sdk *k8s.Sdk) *ResourceMutator {
	scheme := k8s.GetScheme()
	return &ResourceMutator{
		decoder: admission.NewDecoder(scheme),
		client:  client,
		sdk:     sdk,
	}
}

/*
*
[2025-08-20 02:36:16.770]       [ERROR] default user info 处理 admission 请求   {"user": {"username":"system:serviceaccount:default:admin","uid":"9a7aedab-a56b-4742-af51-ff22b2dc8d6d","groups":["system:serviceaccounts","system:serviceaccounts:default","system:authenticated"],"extra":{"authentication.kubernetes.io/credential-id":["JTI=543f2783-1bf9-4b3d-b5ed-ced864a24ad2"]}}}
*/
func (m *ResourceMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
	// slog.Error("处理 admission 请求", slog.String("kind", req.Kind.Kind), slog.String("namespace", req.Namespace), slog.String("name", req.Name),
	// 	slog.String("user", req.UserInfo.Username), slog.String("kindGroup", req.Kind.Group))
	// slog.Error("user info 处理 admission 请求", "user", req.UserInfo)
	// 根据资源类型调用不同的处理函数
	switch req.Kind.Kind {
	case "Service":
		return m.handleService(ctx, req)
	case "StatefulSet":
		return m.handleStatefulSet(ctx, req)
	case "Deployment":
		return m.handleDeployment(ctx, req)
	case "DaemonSet":
		return m.handleDaemonset(ctx, req)
	case "Ingress":
		return m.handleIngress(ctx, req)
	// case "VirtualClusterPolicy":
	// 	return m.handleVirtualClusterPolicy(ctx, req)
	case "Pod":
		return m.handlePod(ctx, req)
	case "Secret":
		return m.handleSecret(ctx, req)
	case "ConfigMap":
		return m.handleConfigmap(ctx, req)
	case "OverSellingConfig":
		return m.handleOverSellingConfig(ctx, req)
	case "McpBridge":
		return m.handleMcpBridge(ctx, req)
	case "Node":
		if req.Kind.Group == "longhorn.io" {
			return m.handleLonghornNode(ctx, req)
		}
		return m.handleNode(ctx, req)
	case "StorageClass":
		return m.handleStorageClass(ctx, req)
	case "Replica":
		if req.Kind.Group == "longhorn.io" {
			return m.handleLonghornReplica(ctx, req)
		}
		return admission.Allowed("不需要修改的资源类型")

	case "PersistentVolumeClaim": //扩容资源时候 删除pod
		return m.handlePvc(ctx, req)
	case "ApiClient":
		return m.handleApiClient(ctx, req)
	case "MicroApp":
		return m.handleMicroApp(ctx, req)
	default:
		return admission.Allowed("不需要修改的资源类型")
	}
}

// 处理 Pod 资源

func setRequestLimit(pod *v1.Pod, cpu resource.Quantity, memory resource.Quantity) bool {

	changed := false
	for j := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[j].Resources.Limits == nil {
			pod.Spec.InitContainers[j].Resources.Limits = make(v1.ResourceList)
		}
		if pod.Spec.InitContainers[j].Resources.Requests == nil {
			pod.Spec.InitContainers[j].Resources.Requests = make(v1.ResourceList)
		}
		limits := pod.Spec.InitContainers[j].Resources.Limits
		if limits.Cpu().IsZero() || limits.Memory().IsZero() {
			pod.Spec.InitContainers[j].Resources.Limits["cpu"] = cpu
			pod.Spec.InitContainers[j].Resources.Limits["memory"] = memory
			changed = true
		}
		if pod.Spec.InitContainers[j].Resources.Requests.Cpu().IsZero() || pod.Spec.InitContainers[j].Resources.Requests.Memory().IsZero() {
			pod.Spec.InitContainers[j].Resources.Requests["cpu"] = resource.MustParse("0")
			pod.Spec.InitContainers[j].Resources.Requests["memory"] = resource.MustParse("0")
			changed = true
		}
	}
	for i := range pod.Spec.Containers {

		if pod.Spec.Containers[i].Resources.Limits == nil {
			pod.Spec.Containers[i].Resources.Limits = make(v1.ResourceList)
		}
		if pod.Spec.Containers[i].Resources.Requests == nil {
			pod.Spec.Containers[i].Resources.Requests = make(v1.ResourceList)
		}
		limits := pod.Spec.Containers[i].Resources.Limits
		if limits.Cpu().IsZero() || limits.Memory().IsZero() {
			pod.Spec.Containers[i].Resources.Limits["cpu"] = cpu
			pod.Spec.Containers[i].Resources.Limits["memory"] = memory
			changed = true
		}
		if pod.Spec.Containers[i].Resources.Requests.Cpu().IsZero() || pod.Spec.Containers[i].Resources.Requests.Memory().IsZero() {
			pod.Spec.Containers[i].Resources.Requests["cpu"] = resource.MustParse("0")
			pod.Spec.Containers[i].Resources.Requests["memory"] = resource.MustParse("0")
			changed = true
		}
	}
	return changed
}
