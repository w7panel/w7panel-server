package installcontroller

import (
	"context"
	"errors"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic"
	api "github.com/w7panel/w7panel/k8s/pkg/apis/zpkinstall/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestZpkInstallClaimConflictHasNoEffects(t *testing.T) {
	r, req := fixture(t, func(context.Context, *api.ZpkInstall) (logic.InstallResult, error) {
		t.Fatal("executed without durable claim")
		return logic.InstallResult{}, nil
	})
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			return apierrors.NewConflict(schema.GroupResource{Group: "w7panel.w7.com", Resource: "zpkinstalls"}, obj.GetName(), errors.New("competing claim"))
		},
	})
	result, err := r.Reconcile(context.Background(), req)
	if err != nil || result.RequeueAfter == 0 {
		t.Fatalf("%+v %v", result, err)
	}
}

func TestZpkInstallLostResultIsNotReplayed(t *testing.T) {
	calls := 0
	r, req := fixture(t, func(context.Context, *api.ZpkInstall) (logic.InstallResult, error) {
		calls++
		return logic.InstallResult{InstallID: "done"}, nil
	})
	r.Client = interceptor.NewClient(r.Client.(client.WithWatch), interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if obj.(*api.ZpkInstall).Status.Phase == "Succeeded" {
				return errors.New("API unavailable")
			}
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
	})
	if _, err := r.Reconcile(context.Background(), req); err == nil {
		t.Fatal("expected status failure")
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(3 * time.Minute)
	r.Now = func() time.Time { return future }
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || getTask(t, r, req).Status.Phase != "Unknown" {
		t.Fatal("lost result must not replay")
	}
}

func TestZpkInstallHeartbeatRenewsClaim(t *testing.T) {
	r, req := fixture(t, nil)
	task := getTask(t, r, req)
	old := metav1.NewTime(time.Now().Add(-time.Minute))
	task.Status = api.ZpkInstallStatus{Phase: "Running", ExecutionID: "active", StartedAt: &old, HeartbeatAt: &old}
	if err := r.Status().Update(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	if err := r.updateOwned(context.Background(), task, func(task *api.ZpkInstall) { task.Status.HeartbeatAt = &now }); err != nil {
		t.Fatal(err)
	}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil || result.RequeueAfter != heartbeatInterval || getTask(t, r, req).Status.Phase != "Running" {
		t.Fatalf("%+v %v", result, err)
	}
}

func fixture(t *testing.T, execute Executor) (*Controller, ctrl.Request) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := api.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	task := &api.ZpkInstall{ObjectMeta: metav1.ObjectMeta{Name: "install", Namespace: "default", UID: "task-uid"},
		Spec: api.ZpkInstallSpec{RepoURL: "https://example.com/app", ReleaseName: "app", InstallOptions: []api.InstallOption{}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&api.ZpkInstall{}).WithObjects(task).Build()
	return &Controller{Client: c, Reader: c, Execute: execute, Now: time.Now}, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(task)}
}

func getTask(t *testing.T, r *Controller, req ctrl.Request) *api.ZpkInstall {
	t.Helper()
	task := &api.ZpkInstall{}
	if err := r.Reader.Get(context.Background(), req.NamespacedName, task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestZpkInstallExecutesOnceAcrossControllers(t *testing.T) {
	var calls atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	r, req := fixture(t, func(ctx context.Context, task *api.ZpkInstall) (logic.InstallResult, error) {
		calls.Add(1)
		close(started)
		<-finish
		return logic.InstallResult{ReleaseName: "actual", Namespace: "helm-ns", InstallID: task.Status.InstallID}, nil
	})
	done := make(chan error, 1)
	go func() { _, err := r.Reconcile(context.Background(), req); done <- err }()
	<-started
	second := *r
	if _, err := second.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	claimed := getTask(t, r, req)
	if claimed.Status.Phase != "Running" || claimed.Status.InstallID == "" {
		t.Fatalf("not durably claimed: %+v", claimed.Status)
	}
	close(finish)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := second.Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	task := getTask(t, r, req)
	if calls.Load() != 1 || task.Status.Phase != "Succeeded" || task.Status.Namespace != "helm-ns" || task.Status.ReleaseName != "actual" {
		t.Fatalf("unexpected result: %d %+v", calls.Load(), task.Status)
	}
}

func TestZpkInstallFailureAndPanicAreTerminalAndRedacted(t *testing.T) {
	for _, panicExec := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicExec], func(t *testing.T) {
			calls := 0
			r, req := fixture(t, func(context.Context, *api.ZpkInstall) (logic.InstallResult, error) {
				calls++
				if panicExec {
					panic("secret-token")
				}
				return logic.InstallResult{}, errors.New("https://secret-token@example.com")
			})
			for i := 0; i < 2; i++ {
				if _, err := r.Reconcile(context.Background(), req); err != nil {
					t.Fatal(err)
				}
			}
			status := getTask(t, r, req).Status
			if calls != 1 || status.Phase != "Failed" || strings.Contains(status.Message, "secret-token") {
				t.Fatalf("%d %+v", calls, status)
			}
		})
	}
}

func TestZpkInstallStaleClaimUnknownAndLateResultRejected(t *testing.T) {
	r, req := fixture(t, func(context.Context, *api.ZpkInstall) (logic.InstallResult, error) {
		t.Fatal("must not replay")
		return logic.InstallResult{}, nil
	})
	task := getTask(t, r, req)
	old := metav1.NewTime(time.Now().Add(-3 * time.Minute))
	task.Status = api.ZpkInstallStatus{Phase: "Running", ExecutionID: "old-worker", StartedAt: &old, HeartbeatAt: &old}
	if err := r.Status().Update(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	claimed := task.DeepCopy()
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if getTask(t, r, req).Status.Phase != "Unknown" {
		t.Fatal("expected Unknown")
	}
	if err := r.updateOwned(context.Background(), claimed, func(task *api.ZpkInstall) { task.Status.Phase = "Succeeded" }); err == nil {
		t.Fatal("accepted late result")
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestZpkInstallSecretScopeAndRequestMapping(t *testing.T) {
	r, req := fixture(t, nil)
	ref := &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "repo"}, Key: "token"}
	foreign := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "repo", Namespace: "other"}, Data: map[string][]byte{"token": []byte("private")}}
	if err := r.Create(context.Background(), foreign); err != nil {
		t.Fatal(err)
	}
	if _, err := r.secretValue(context.Background(), "default", ref); err == nil {
		t.Fatal("cross-namespace secret read")
	}
	foreign.Namespace = "default"
	foreign.ResourceVersion = ""
	foreign.UID = ""
	if err := r.Create(context.Background(), foreign); err != nil {
		t.Fatal(err)
	}
	value, err := r.secretValue(context.Background(), "default", ref)
	if err != nil || value != "private" {
		t.Fatalf("secret resolution: %v", err)
	}
	task := getTask(t, r, req)
	task.Spec.IsTrandition = true
	task.Spec.ZipURL = "https://example.com/code.zip"
	task.Spec.Reinstall = true
	task.Spec.InstallOptions = []api.InstallOption{{Identifie: "app", Replicas: 1, EnvKv: []api.EnvKv{{Name: "MODE", Value: "test"}}}}
	request, err := requestFor(task)
	if err != nil || request.Namespace != "default" || !request.IsTrandition || !request.Reinstall || request.InstallOptions[0].EnvKv[0].Value != "test" {
		t.Fatalf("mapping: %+v %v", request, err)
	}
	if request.InstallOptions[0].RealToken != "" || request.InstallOptions[0].K8sToken != nil || request.InstallOptions[0].ServiceAccountName != "" {
		t.Fatal("runtime identity leaked into request")
	}
}

func TestZpkInstallDeleteDoesNotOwnApplications(t *testing.T) {
	r, req := fixture(t, func(context.Context, *api.ZpkInstall) (logic.InstallResult, error) { return logic.InstallResult{}, nil })
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	task := getTask(t, r, req)
	if len(task.Finalizers) != 0 {
		t.Fatal("task must not uninstall applications")
	}
	if err := r.Delete(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
}

func TestZpkInstallReplacedTaskRejectsOldWorker(t *testing.T) {
	r, req := fixture(t, nil)
	old := getTask(t, r, req)
	old.Status = api.ZpkInstallStatus{Phase: "Running", ExecutionID: "worker"}
	if err := r.Status().Update(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	replacement := old.DeepCopy()
	replacement.ResourceVersion = ""
	replacement.UID = types.UID("new-uid")
	if err := r.Create(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	if err := r.updateOwned(context.Background(), old, func(*api.ZpkInstall) {}); err == nil {
		t.Fatal("accepted old worker")
	}
}
