package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	appgroupv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeArtifactInstaller struct {
	lookupCalls    int
	installCalls   int
	uninstallCalls int
	installed      *installedArtifact
	err            error
}

func (f *fakeArtifactInstaller) Lookup(context.Context, *installationv1.BootstrapInstallation) (*installedArtifact, error) {
	f.lookupCalls++
	return f.installed, f.err
}
func (f *fakeArtifactInstaller) Install(context.Context, *installationv1.BootstrapInstallation) error {
	f.installCalls++
	return f.err
}
func (f *fakeArtifactInstaller) Uninstall(context.Context, *installationv1.BootstrapInstallation) error {
	f.uninstallCalls++
	return f.err
}

func bootstrapTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := installationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := appgroupv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func TestLeaseSlotsLimitConcurrency(t *testing.T) {
	slots := newLeaseSlots(fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).Build(), "default")
	ctx := context.Background()
	acquired, err := slots.acquire(ctx, bootstrapSlotScope, "operation-one", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	acquired, err = slots.acquire(ctx, bootstrapSlotScope, "operation-two", 1, time.Minute)
	if err != nil || acquired {
		t.Fatalf("second acquire = %v, %v; want busy", acquired, err)
	}
	if err := slots.release(ctx, "operation-one"); err != nil {
		t.Fatal(err)
	}
	acquired, err = slots.acquire(ctx, bootstrapSlotScope, "operation-two", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire after release = %v, %v", acquired, err)
	}
}

func TestInstallationReconcilerInstallsDirectResource(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 1 {
		t.Fatalf("install calls = %d, want 1", installer.installCalls)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: item.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhaseInstalling {
		t.Fatalf("unexpected status: %#v", updated.Status)
	}
}

func TestInstallationDeletionUninstallsApplication(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	if _, err := reconciler.reconcileDeletion(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if installer.uninstallCalls != 1 {
		t.Fatalf("uninstall calls = %d, want 1", installer.uninstallCalls)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: item.Name}, updated); err != nil {
		t.Fatal(err)
	}
	for _, finalizer := range updated.Finalizers {
		if finalizer == installationv1.InstallationFinalizer {
			t.Fatalf("finalizer was not removed: %v", updated.Finalizers)
		}
	}
}

func TestAvailableAppGroupMarksInstallationReadyAndStops(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{Name: "w7panel-higress", Namespace: "default", Identifie: "w7panel-higress", State: installedArtifactReady}}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", installer.installCalls)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("ready installation must stop reconciling: %#v", result)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: item.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhaseReady || updated.Status.CompletedAt == nil {
		t.Fatalf("unexpected ready status: %#v", updated.Status)
	}
	if installer.lookupCalls != 1 {
		t.Fatalf("lookup calls = %d, want 1", installer.lookupCalls)
	}
}

func TestReadyInstallationStopsWithoutLookingUpAppGroup(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Status.Phase = installationv1.BootstrapPhaseReady
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", installer.installCalls)
	}
	if installer.lookupCalls != 0 {
		t.Fatalf("lookup calls = %d, want 0", installer.lookupCalls)
	}
}

func TestFailedApplicationAtRetryLimitRemainsIdle(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Status = installationv1.BootstrapInstallationStatus{
		Phase:      installationv1.BootstrapPhaseFailed,
		RetryCount: defaultMaxRetries,
		Message:    "AppGroup 安装失败",
	}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "w7panel-higress", Namespace: "default", State: installedArtifactFailed, Owned: true,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requeue || result.RequeueAfter != 0 || installer.installCalls != 0 || installer.uninstallCalls != 0 {
		t.Fatalf("unexpected terminal retry activity: result=%#v installs=%d uninstalls=%d", result, installer.installCalls, installer.uninstallCalls)
	}
}

func TestFailedApplicationRetriesAfterMaxRetriesIncreases(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Status = installationv1.BootstrapInstallationStatus{
		Phase:      installationv1.BootstrapPhaseFailed,
		RetryCount: defaultMaxRetries,
	}
	item.Spec.Strategy.MaxRetries = ptr.To(defaultMaxRetries + 1)
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "w7panel-higress", Namespace: "default", State: installedArtifactFailed, Owned: true,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	if installer.uninstallCalls != 1 {
		t.Fatalf("uninstall calls = %d, want 1", installer.uninstallCalls)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: item.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhasePending || updated.Status.RetryCount != defaultMaxRetries+1 {
		t.Fatalf("unexpected retry status: %#v", updated.Status)
	}
}

func TestFailedApplicationIsReinstalledWhenAppGroupDisappears(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Status = installationv1.BootstrapInstallationStatus{
		Phase:      installationv1.BootstrapPhaseFailed,
		RetryCount: defaultMaxRetries,
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 1 {
		t.Fatalf("install calls = %d, want 1", installer.installCalls)
	}
}

func TestFailedOwnedApplicationIsDeletedBeforeRetry(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Status = installationv1.BootstrapInstallationStatus{
		Phase:       installationv1.BootstrapPhaseInstalling,
		OperationID: "failed-operation",
		StartedAt:   ptr.To(metav1.Now()),
	}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "w7panel-higress", Namespace: "default", Identifie: "w7panel-higress",
		State: installedArtifactFailed, Owned: true,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if installer.uninstallCalls != 1 {
		t.Fatalf("uninstall calls = %d, want 1", installer.uninstallCalls)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), request.NamespacedName, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhasePending || updated.Status.RetryCount != 1 {
		t.Fatalf("unexpected retry status: %#v", updated.Status)
	}

	installer.installed = nil
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 1 {
		t.Fatalf("install calls after cleanup = %d, want 1", installer.installCalls)
	}
}

func TestFailedForeignApplicationIsNotDeletedForRetry(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "w7panel-higress", Namespace: "default", Identifie: "w7panel-higress",
		State: installedArtifactFailed, Owned: false,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if installer.uninstallCalls != 0 {
		t.Fatalf("uninstall calls = %d, want 0", installer.uninstallCalls)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), request.NamespacedName, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhaseFailed {
		t.Fatalf("unexpected foreign application status: %#v", updated.Status)
	}
}

func TestRetryAllowsConfiguredNumberOfRetries(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient}
	settings := installationSettings(item)
	key := types.NamespacedName{Name: item.Name}

	for failure := int32(1); failure <= settings.MaxRetries; failure++ {
		current := &installationv1.BootstrapInstallation{}
		if err := k8sClient.Get(context.Background(), key, current); err != nil {
			t.Fatal(err)
		}
		if _, err := reconciler.retry(context.Background(), current, settings, errors.New("install failed")); err != nil {
			t.Fatal(err)
		}
		if err := k8sClient.Get(context.Background(), key, current); err != nil {
			t.Fatal(err)
		}
		if current.Status.Phase != installationv1.BootstrapPhasePending || current.Status.RetryCount != failure {
			t.Fatalf("failure %d status = %#v", failure, current.Status)
		}
	}

	current := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), key, current); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.retry(context.Background(), current, settings, errors.New("install failed")); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Get(context.Background(), key, current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != installationv1.BootstrapPhaseFailed || current.Status.RetryCount != settings.MaxRetries {
		t.Fatalf("terminal retry status = %#v", current.Status)
	}
}

func TestInvalidInstallationDoesNotInstall(t *testing.T) {
	item := validInstallation()
	item.Finalizers = []string{installationv1.InstallationFinalizer}
	item.Spec.Artifact.Source = "http://zpk.w7.cc/info/higress"
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(bootstrapTestScheme(t)).WithStatusSubresource(item).WithObjects(item).Build()
	reconciler := &InstallationReconciler{Client: k8sClient, Scheme: bootstrapTestScheme(t), installer: installer, slots: newLeaseSlots(k8sClient, "default")}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	updated := &installationv1.BootstrapInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: item.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != installationv1.BootstrapPhaseFailed || installer.installCalls != 0 {
		t.Fatalf("unexpected invalid-resource result: status=%#v installs=%d", updated.Status, installer.installCalls)
	}

	updated.Spec.Artifact.Source = validInstallation().Spec.Artifact.Source
	if err := k8sClient.Update(context.Background(), updated); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: item.Name}}); err != nil {
		t.Fatal(err)
	}
	if installer.installCalls != 1 {
		t.Fatalf("install calls after fixing resource = %d, want 1", installer.installCalls)
	}
}

func TestArtifactOwnershipRequiresInstallationUID(t *testing.T) {
	item := validInstallation()
	direct := map[string]string{installationv1.AnnotationInstallationOwner: artifactOwner(item)}
	if !isArtifactOwner(direct, item) {
		t.Fatal("direct ownership was not recognized")
	}
	legacy := map[string]string{installationv1.AnnotationInstallationOwner: "legacy-profile-uid/higress"}
	if isArtifactOwner(legacy, item) {
		t.Fatal("legacy profile ownership must not be accepted")
	}
}

func TestAppGroupArtifactStateRequiresReadyAndDeployed(t *testing.T) {
	tests := []struct {
		name  string
		group *appgroupv1.AppGroup
		want  installedArtifactState
	}{
		{name: "ready", group: &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeployed}}, want: installedArtifactReady},
		{name: "installing", group: &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeploying}}, want: installedArtifactInstalling},
		{name: "failed", group: &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{DeployStatus: appgroupv1.StatusFailed}}, want: installedArtifactFailed},
		{name: "deleting", group: &appgroupv1.AppGroup{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}}, Status: appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeployed}}, want: installedArtifactDeleting},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := appGroupArtifactState(test.group); got != test.want {
				t.Fatalf("state = %q, want %q", got, test.want)
			}
		})
	}
}
