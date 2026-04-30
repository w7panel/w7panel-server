package k3k

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	k3kv1 "github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/k3k/overselling"
	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	cvmv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/cvm/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func CvmToK3kConfig(cvm *cvmv1alpha1.Cvm) *k8s.K3kConfig {
	return k8s.NewK3kConfig(strings.ReplaceAll(cvm.Namespace, "k3k-", ""), cvm.Namespace, "", cvm.Name)
}

const (
	K3kCvmFinalizerName = "cvm.k3k.io/finalizer"
	K3kCvmNameLabel     = "w7.cc/cvm-name"
	K3kCvmNamespaceAnno = "w7.cc/cvm-namespace"

	capacityCheckStatePending    = "pending"
	capacityCheckStateWait       = "wait"
	capacityCheckStateSuccess    = "success"
	capacityCheckStateNoResource = "no-resource"
)

type K3kCvmController struct {
	client.Client
	Scheme *runtime.Scheme
	Sdk    *k8s.Sdk
}

func setupCvmController(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	r := &K3kCvmController{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		Sdk:    sdk,
	}

	cvmPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "create", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "delete", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "generic", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, okOld := e.ObjectOld.(*cvmv1alpha1.Cvm)
			newObj, okNew := e.ObjectNew.(*cvmv1alpha1.Cvm)
			if !okOld || !okNew {
				return true
			}
			if oldObj.GetGeneration() != newObj.GetGeneration() {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "update", "reason", "generation", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldObj.GetDeletionTimestamp(), newObj.GetDeletionTimestamp()) {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "update", "reason", "deletionTimestamp", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldObj.GetFinalizers(), newObj.GetFinalizers()) {
				slog.Info("K3kCvm enqueue", "source", "cvm", "event", "update", "reason", "finalizers", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			return false
		},
	}

	clusterPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "create", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "delete", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			if e.Object != nil {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "generic", "namespace", e.Object.GetNamespace(), "name", e.Object.GetName())
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldObj, okOld := e.ObjectOld.(*k3kv1.Cluster)
			newObj, okNew := e.ObjectNew.(*k3kv1.Cluster)
			if !okOld || !okNew {
				return true
			}
			if oldObj.GetGeneration() != newObj.GetGeneration() {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "update", "reason", "generation", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldObj.GetDeletionTimestamp(), newObj.GetDeletionTimestamp()) {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "update", "reason", "deletionTimestamp", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldObj.GetFinalizers(), newObj.GetFinalizers()) {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "update", "reason", "finalizers", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			if oldObj.Status.Phase != newObj.Status.Phase {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "update", "reason", "phase", "namespace", newObj.Namespace, "name", newObj.Name, "old", oldObj.Status.Phase, "new", newObj.Status.Phase)
				return true
			}
			if !apiequality.Semantic.DeepEqual(oldObj.Status.Conditions, newObj.Status.Conditions) {
				slog.Info("K3kCvm enqueue", "source", "cluster", "event", "update", "reason", "conditions", "namespace", newObj.Namespace, "name", newObj.Name)
				return true
			}
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&cvmv1alpha1.Cvm{}, ctrlbuilder.WithPredicates(cvmPredicate)).
		Owns(&k3kv1.Cluster{}, ctrlbuilder.WithPredicates(clusterPredicate)).
		// Owns(&batchv1.Job{}). //job 在其他命名空间 没有service-account 只能在default 所有不能owns
		Complete(r)
}

func (r *K3kCvmController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	result, err := r.reconcile0(ctx, req)
	if err != nil {
		stack := debug.Stack()
		slog.Error("K3kCvm reconcile error",
			"error_message", err.Error(),
			"stack_trace", string(stack),
			"error_type", fmt.Sprintf("%T", err),
			"name", req.Name,
			"namespace", req.Namespace,
		)
	}
	return result, err
}

func (r *K3kCvmController) reconcile0(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in K3kCvm Handle", "panic", r)
		}
	}()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling Cvm", "namespace", req.Namespace, "name", req.Name)

	cvm := &cvmv1alpha1.Cvm{}
	if err := r.Get(ctx, req.NamespacedName, cvm); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Cvm")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}

	if !cvm.DeletionTimestamp.IsZero() {
		logger.Info("Cvm is being deleted", "namespace", req.Namespace, "name", req.Name)
		return r.handleDeletion(ctx, cvm)
	}

	if !controllerutil.ContainsFinalizer(cvm, K3kCvmFinalizerName) {
		logger.Info("Adding finalizer", "namespace", req.Namespace, "name", req.Name)
		controllerutil.AddFinalizer(cvm, K3kCvmFinalizerName)
		if err := r.Update(ctx, cvm); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		return ctrl.Result{}, nil
	}
	err := r.checkResource(ctx, cvm)
	if err != nil {
		logger.Error(err, "Failed to check resource")
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}
	if err := r.reconcileResourceStatus(ctx, cvm); err != nil {
		logger.Error(err, "Failed to reconcile effective resource")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	// 救援模式
	if !*cvm.Status.IsExpired && cvm.Spec.Rescue {
		err := r.doRescue(ctx, cvm)
		if err != nil {
			slog.Error("do rescue err", "err", err)
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}
	if cvm.IsEmpty() || *cvm.Status.IsExpired { //TODO 演示用户过期立即删除 否则回收才删除cluster
		// 过期后直接删除cluster
		cluster := &k3kv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cvm.Name,
				Namespace: cvm.Namespace,
			},
		}
		if err := r.Delete(ctx, cluster); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
		pvcName := cvm.GetClusterServer0PvcName()
		if pvcName != "" {
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pvcName,
					Namespace: cvm.Namespace,
				},
			}
			if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil //TODO 重试
			}

		}
		//TODO : cluster 删除需要一并删除pvc
		return ctrl.Result{}, nil
	}

	cluster, err := r.createOrUpdateCluster(ctx, cvm)
	if err != nil {
		logger.Error(err, "Failed to create/update k3k Cluster")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if err := r.syncClusterStatus(ctx, cvm, cluster); err != nil {
		logger.Error(err, "Failed to sync cluster status to cvm")
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if string(cvm.Status.ClusterPhase) == string(k3kv1.ClusterReady) {
		if err := r.createAgent(ctx, cvm); err != nil {
			logger.Error(err, "Failed to create agent1x")
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		}
	}

	return ctrl.Result{}, nil
}

// 执行救援
func (r *K3kCvmController) doRescue(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {
	job := k3ktypes.ToK3kWeihJob(cvm)
	// err := controllerutil.SetControllerReference(cvm, job, r.Scheme)
	// if err != nil {
	// 	return err
	// }
	err := r.Client.Create(ctx, job)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}
	// job 只能default namespace 所以cvm 不能实时触发
	// dbJob := &batchv1.Job{}
	// err = r.Client.Get(ctx, client.ObjectKeyFromObject(job), dbJob)
	// if err != nil {
	// 	if !apierrors.IsNotFound(err) {
	// 		return err
	// 	}
	// }
	// if err == nil {
	// 	base := cvm.DeepCopy()
	// 	cvm.ComputeStatus()
	// 	phasa := "running"
	// 	if job.Status.Succeeded > 1 {
	// 		phasa = "success"
	// 	}
	// 	if job.Status.Failed > 1 {
	// 		phasa = "failed"
	// 	}
	// 	cvm.Status.RescuePhase = phasa
	// 	if !apiequality.Semantic.DeepEqual(cvm.Status, base.Status) {
	// 		err = r.Status().Patch(ctx, cvm, client.MergeFrom(base))
	// 		if err != nil {
	// 			return err
	// 		}
	// 	}
	// }
	return nil
}

func (r *K3kCvmController) createOrUpdateCluster(ctx context.Context, cvm *cvmv1alpha1.Cvm) (*k3kv1.Cluster, error) {
	cluster := &k3kv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cvm.Name,
			Namespace: cvm.Namespace,
		},
	}
	result, err := controllerutil.CreateOrPatch(ctx, r.Client, cluster, func() error {
		desiredLabels := map[string]string{
			"cvm-uid": string(cvm.UID),
		}
		if !apiequality.Semantic.DeepEqual(cluster.Labels, desiredLabels) {
			// slog.Info("Cluster labels changed",
			// 	"name", cluster.Name,
			// 	"namespace", cluster.Namespace,
			// 	"current", cluster.Labels,
			// 	"desired", desiredLabels,
			// )
			cluster.Labels = desiredLabels
		}
		desiredSpec := r.toClusterSpec(cvm)
		if !cluster.CreationTimestamp.IsZero() && cluster.Spec.Persistence.StorageRequestSize != "" {
			// K3K does not support resizing the backing persistence request after the cluster exists.
			desiredSpec.Persistence.StorageRequestSize = cluster.Spec.Persistence.StorageRequestSize
		}
		if !apiequality.Semantic.DeepEqual(cluster.Spec, desiredSpec) {
			// slog.Info("Cluster spec changed",
			// 	"name", cluster.Name,
			// 	"namespace", cluster.Namespace,
			// 	"current", cluster.Spec,
			// 	"desired", desiredSpec,
			// )
			cluster.Spec = desiredSpec
		}
		return controllerutil.SetControllerReference(cvm, cluster, r.Scheme)
	})
	slog.Info("Cluster", "result", result, "cluster", cluster)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (r *K3kCvmController) toClusterSpec(cvm *cvmv1alpha1.Cvm) k3kv1.ClusterSpec {
	servers := int32(1)
	agents := int32(0)
	effective := desiredEffectiveResource(cvm)
	spec := k3kv1.ClusterSpec{
		Servers: &servers,
		Agents:  &agents,
		Persistence: k3kv1.PersistenceConfig{
			Type:               k3kv1.DynamicPersistenceMode,
			StorageRequestSize: "1G",
		},
		Sync: &k3kv1.SyncConfig{
			Services: k3kv1.ServiceSyncConfig{
				Enabled: true,
			},
			ConfigMaps: k3kv1.ConfigMapSyncConfig{
				Enabled: true,
			},
			Secrets: k3kv1.SecretSyncConfig{
				Enabled: true,
			},
			Ingresses: k3kv1.IngressSyncConfig{
				Enabled: false,
			},
			PersistentVolumeClaims: k3kv1.PersistentVolumeClaimSyncConfig{
				Enabled: true,
			},
			PriorityClasses: k3kv1.PriorityClassSyncConfig{
				Enabled: false,
			},
		},
	}
	spec.Mode = k3kv1.VirtualClusterMode
	// serverArgs:
	// - '--kubelet-arg=$cgroup_root'
	// - '--disable=traefik'
	// - '--embedded-registry'
	// - '--disable-network-policy'
	spec.ServerArgs = []string{
		"--kubelet-arg=$cgroup_root",
		"--disable=traefik",
		"--embedded-registry",
		"--disable-network-policy",
		"--etcd-arg=quota-backend-bytes=5368709120",
	}
	spec.ServerEnvs = []v1.EnvVar{
		{
			Name: "GOMAXPROCS",
			ValueFrom: &v1.EnvVarSource{
				ResourceFieldRef: &v1.ResourceFieldSelector{
					Divisor:  resource.MustParse("1"),
					Resource: "limits.cpu",
				},
			},
		},
		{
			Name: "K3K_HOST_IP",
			ValueFrom: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					APIVersion: "v1",
					FieldPath:  "status.hostIP",
				},
			},
		},
		{
			Name:  "TZ",
			Value: "Asia/Shanghai",
		},
	}
	//https://rancher.github.io/k3k-product-docs/k3k/1.0.2/en/how-tos-for-user/addons.html
	// 0.3.5 还是是用v1alpha1 导致暂时无法升级到1.0.1
	// spec.Addons = []k3kv1.Addon{
	// 	{
	// 		SecretNamespace: "default",
	// 		SecretRef:       "k3k.addon",
	// 	},
	// }
	// 测试暂时去掉 limit
	// spec.ServerLimit = v1.ResourceList{
	// 	v1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", effective.CPU)),
	// 	v1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dGi", effective.Memory)),
	// }

	if cvm.Spec.StorageClassName != "" && effective != nil && effective.Storage > 0 {
		spec.Persistence = k3kv1.PersistenceConfig{
			StorageClassName:   &cvm.Spec.StorageClassName,
			Type:               k3kv1.DynamicPersistenceMode,
			StorageRequestSize: fmt.Sprintf("%dGi", effective.Storage),
		}
	}
	return spec
}

func (r *K3kCvmController) handleDeletion(ctx context.Context, cvm *cvmv1alpha1.Cvm) (ctrl.Result, error) {
	slog.Info("Handling Cvm deletion", "name", cvm.Name, "namespace", cvm.Namespace)

	cluster := &k3kv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cvm.Name,
			Namespace: cvm.Namespace,
		},
	}
	if err := r.Delete(ctx, cluster); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if err := r.ensureClusterDeleted(ctx, cvm); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	controllerutil.RemoveFinalizer(cvm, K3kCvmFinalizerName)
	if err := r.Update(ctx, cvm); err != nil {
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}

	return ctrl.Result{}, nil
}

// no-resource 无可用资源 wait 待处理 success 资源检查通过
func (r *K3kCvmController) checkResource(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {
	if cvm.Spec.CapacityCheckState == capacityCheckStateWait || cvm.Spec.CapacityCheckState == capacityCheckStateNoResource {
		// 检查资源是否充足
		err := overselling.CanAddResourceCvm(cvmResourceToOverSelling(cvm.Spec.PendingPurchasedResource), getCvmResource)
		hasRs := false
		if err == nil {
			hasRs = true
		}
		_, err = controllerutil.CreateOrPatch(ctx, r.Client, cvm, func() error {
			if hasRs {
				cvm.CheckSuccess()
				return nil
			}
			cvm.CheckNoResource()
			return nil
		})
		return err
	}
	return nil
}

func (r *K3kCvmController) ensureClusterDeleted(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {
	cluster := &k3kv1.Cluster{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      cvm.Name,
		Namespace: cvm.Namespace,
	}, cluster)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *K3kCvmController) syncClusterStatus(ctx context.Context, cvm *cvmv1alpha1.Cvm, cluster *k3kv1.Cluster) error {
	refreshCluster := func() error {
		return r.Get(ctx, client.ObjectKey{
			Name:      cvm.Name,
			Namespace: cvm.Namespace,
		}, cluster)
	}
	err := refreshCluster()
	if err != nil {
		if apierrors.IsNotFound(err) {

			return nil
		}
		return err
	}
	base := cvm.DeepCopy()
	cvm.Status.ClusterPhase = k3kv1.ClusterPhase(cluster.Status.Phase)
	cvm.Status.Conditions = cluster.Status.Conditions
	cvm.ComputeStatus()
	if apiequality.Semantic.DeepEqual(cvm.Status, base.Status) {
		return nil
	}
	return r.Status().Patch(ctx, cvm, client.MergeFrom(base))
}

func (r *K3kCvmController) reconcileResourceStatus(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {
	base := cvm.DeepCopy()
	cvm.ComputeStatus()
	if apiequality.Semantic.DeepEqual(cvm.Status, base.Status) {
		return nil
	}
	return r.Status().Patch(ctx, cvm, client.MergeFrom(base))

}

func addResources(left, right *cvmv1alpha1.CvmResource) *cvmv1alpha1.CvmResource {
	if left == nil && right == nil {
		return nil
	}
	sum := &cvmv1alpha1.CvmResource{}
	if left != nil {
		sum.CPU += left.CPU
		sum.Memory += left.Memory
		sum.Storage += left.Storage
		sum.Bandwidth += left.Bandwidth
	}
	if right != nil {
		sum.CPU += right.CPU
		sum.Memory += right.Memory
		sum.Storage += right.Storage
		sum.Bandwidth += right.Bandwidth
	}
	if isZeroResource(sum) {
		return nil
	}
	return sum
}

func isZeroResource(resource *cvmv1alpha1.CvmResource) bool {
	return resource == nil || (resource.CPU == 0 && resource.Memory == 0 && resource.Storage == 0 && resource.Bandwidth == 0)
}

func desiredEffectiveResource(cvm *cvmv1alpha1.Cvm) *cvmv1alpha1.CvmResource {
	return addResources(cvm.Spec.UserResource, cvm.Spec.PurchasedResource)
}

func (r *K3kCvmController) createAgent(ctx context.Context, cvm *cvmv1alpha1.Cvm) error {

	root := k8s.NewK8sClient()
	config := CvmToK3kConfig(cvm)
	clientSdk, err := root.GetK3kClusterSdkByConfig0(config, false)
	if err != nil {
		slog.Warn("failed to get sdk", "err", err)
		return err
	}
	clientSigClient, err := clientSdk.ToSigClient()
	if err != nil {
		slog.Warn("failed to get sigclient", "err", err)
		return err
	}
	sa := k3ktypes.ToServiceAccount(cvm)
	_, err = controllerutil.CreateOrUpdate(ctx, clientSigClient, sa, func() error { return nil })
	if err != nil {
		slog.Warn("failed to create sa", "err", err)
		return err
	}
	rolebings := k3ktypes.ToClusterRoleBinding(cvm)
	_, err = controllerutil.CreateOrUpdate(ctx, clientSigClient, rolebings, func() error { return nil })
	if err != nil {
		slog.Warn("failed to create rolebings", "err", err)
		return err
	}
	// 子集群service
	agentService := k3ktypes.ToK3kAgentService(cvm)
	_, err = controllerutil.CreateOrUpdate(ctx, clientSigClient, agentService, func() error { return nil })
	if err != nil {
		slog.Warn("failed to create agentService", "err", err)
		return err
	}
	//主集群入口service
	ingService := k3ktypes.ToVirtualIngressService(cvm)
	clone := ingService.DeepCopy()
	_, err = controllerutil.CreateOrPatch(ctx, r.Client, clone, func() error {
		clone.Spec = ingService.Spec
		return nil
	})
	if err != nil {
		slog.Warn("failed to create ingService", "err", err)
		return err
	}

	ds := k3ktypes.ToK3kDaemonSet(cvm)
	copy := ds.DeepCopy()
	_, err = controllerutil.CreateOrPatch(ctx, clientSigClient, copy, func() error {
		//copy 变成 etcd 返回的 ds
		copy.Annotations = ds.Annotations
		// host-ip helm-version 任意一个变动就patch 更新 否则 不更新
		copy.Annotations["root-node-ip"] = os.Getenv("NODE_IP") //
		copy.Annotations["helm-version"] = os.Getenv("HELM_VERSION")
		// copy.Labels["d"]
		copy.Spec = ds.Spec
		return nil
	})
	if err != nil {
		slog.Warn("failed to create daemonSet", "err", err)
		return err
	}
	// slog.Error("create agent daemonset", "result", result, "name", k3kUser.GetName())
	// helmVersion := os.Getenv("HELM_VERSION") //pod.Annotations["helm-version"]
	// podVersion := pod.Annotations["helm-version"]
	// rootPodIp := pod.Annotations["root-pod-ip"]
	// needReCreate := helmVersion != podVersion && helmVersion != "" || rootPodIp != os.Getenv("ROOT_POD_IP")
	// // If pod is in failed state, delete and recreate it
	// if pod.Status.Phase == corev1.PodUnknown || pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed || needReCreate {
	// 	if err := clientSigClient.Delete(ctx, pod); err != nil {
	// 		slog.Warn("failed to delete pod", "err", err)
	// 		return err
	// 	}
	// 	return r.createPod(ctx, clientSigClient, k3kUser)
	// }
	return nil
}
