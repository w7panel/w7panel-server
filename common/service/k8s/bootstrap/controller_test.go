package bootstrap

import (
	"context"
	"testing"
	"time"

	appgroupv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeArtifactInstaller struct {
	installCalls   int
	uninstallCalls int
	lookupCalls    int
	installed      *installedArtifact
	lookupErr      error
	err            error
	installTimeout time.Duration
	waitForCancel  bool
}

func (f *fakeArtifactInstaller) Lookup(context.Context, *bootstrapv1.ArtifactInstallation) (*installedArtifact, error) {
	f.lookupCalls++
	return f.installed, f.lookupErr
}

func (f *fakeArtifactInstaller) Install(ctx context.Context, _ *bootstrapv1.ArtifactInstallation) error {
	f.installCalls++
	if deadline, ok := ctx.Deadline(); ok {
		f.installTimeout = time.Until(deadline)
	}
	if f.waitForCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.err
}

func (f *fakeArtifactInstaller) Uninstall(context.Context, *bootstrapv1.ArtifactInstallation) error {
	f.uninstallCalls++
	return f.err
}

func bootstrapTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := bootstrapv1.AddToScheme(scheme); err != nil {
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

	acquired, err := slots.acquire(ctx, "profile", "operation-one", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	acquired, err = slots.acquire(ctx, "profile", "operation-two", 1, time.Minute)
	if err != nil || acquired {
		t.Fatalf("second acquire = %v, %v; want busy", acquired, err)
	}
	if err := slots.release(ctx, "operation-one"); err != nil {
		t.Fatal(err)
	}
	acquired, err = slots.acquire(ctx, "profile", "operation-two", 1, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire after release = %v, %v", acquired, err)
	}
}

func TestAppGroupArtifactStateRequiresReadyAndDeployed(t *testing.T) {
	tests := []struct {
		name   string
		group  *appgroupv1.AppGroup
		wanted installedArtifactState
	}{
		{
			name:   "ready and deployed",
			group:  &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeployed}},
			wanted: installedArtifactReady,
		},
		{
			name:   "ready flag before deploy completion",
			group:  &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeploying}},
			wanted: installedArtifactInstalling,
		},
		{
			name:   "deployed before workloads ready",
			group:  &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{Ready: false, DeployStatus: appgroupv1.StatusDeployed}},
			wanted: installedArtifactInstalling,
		},
		{
			name:   "failed",
			group:  &appgroupv1.AppGroup{Status: appgroupv1.AppGroupStatus{DeployStatus: appgroupv1.StatusFailed}},
			wanted: installedArtifactFailed,
		},
		{
			name: "deleting takes precedence",
			group: &appgroupv1.AppGroup{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}},
				Status:     appgroupv1.AppGroupStatus{Ready: true, DeployStatus: appgroupv1.StatusDeployed},
			},
			wanted: installedArtifactDeleting,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := appGroupArtifactState(test.group); got != test.wanted {
				t.Fatalf("appGroupArtifactState() = %q, want %q", got, test.wanted)
			}
		})
	}
}

func TestProfileReconcilerSynchronizesAllArtifacts(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision: "1.0.0-1",
			Artifacts: []bootstrapv1.BootstrapArtifact{
				{Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default"},
				{Name: "two", Identifie: "two", Source: "https://zpk.w7.cc/info/two", ReleaseName: "two", Namespace: "default"},
				{Name: "three", Identifie: "three", Source: "https://zpk.w7.cc/info/three", ReleaseName: "three", Namespace: "default"},
			},
		},
	}
	existing := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:   artifactInstallationName(profile.Name, "two"),
			Labels: map[string]string{bootstrapv1.LabelProfile: profile.Name, bootstrapv1.LabelArtifact: "two"},
		},
		Spec: effectiveArtifact(profile, profile.Spec.Artifacts[1]),
	}
	obsolete := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:       artifactInstallationName(profile.Name, "obsolete"),
			Labels:     map[string]string{bootstrapv1.LabelProfile: profile.Name, bootstrapv1.LabelArtifact: "obsolete"},
			Finalizers: []string{bootstrapv1.ArtifactFinalizer},
		},
		Spec: bootstrapv1.ArtifactInstallationSpec{
			ProfileRef: bootstrapv1.BootstrapProfileReference{Name: profile.Name, UID: string(profile.UID)},
			Artifact:   bootstrapv1.ArtifactReference{Name: "obsolete"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(profile, &bootstrapv1.ArtifactInstallation{}).WithObjects(profile, existing, obsolete).Build()
	reconciler := &ProfileReconciler{Client: k8sClient, Scheme: scheme}
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: profile.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected a status refresh after synchronizing resources")
	}
	list := &bootstrapv1.ArtifactInstallationList{}
	if err := k8sClient.List(context.Background(), list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 4 {
		t.Fatalf("installations = %d, want 3 desired artifacts plus terminating obsolete artifact", len(list.Items))
	}
	for _, artifact := range profile.Spec.Artifacts {
		installation := &bootstrapv1.ArtifactInstallation{}
		name := artifactInstallationName(profile.Name, artifact.Name)
		if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: name}, installation); err != nil {
			t.Fatalf("get desired installation %q: %v", name, err)
		}
		if len(installation.Finalizers) != 1 || installation.Finalizers[0] != bootstrapv1.ArtifactFinalizer {
			t.Fatalf("desired installation %q finalizers = %v", name, installation.Finalizers)
		}
	}
	removed := &bootstrapv1.ArtifactInstallation{}
	removedName := artifactInstallationName(profile.Name, "obsolete")
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: removedName}, removed); err != nil {
		t.Fatalf("get obsolete installation %q: %v", removedName, err)
	}
	if removed.DeletionTimestamp.IsZero() {
		t.Fatalf("obsolete installation removal was not requested: %#v", removed.ObjectMeta)
	}
}

func TestReconcileDeletionUsesArtifactInstaller(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "default-profile-one",
			Finalizers: []string{bootstrapv1.ArtifactFinalizer},
		},
		Spec: bootstrapv1.ArtifactInstallationSpec{
			Target: bootstrapv1.ArtifactTarget{ReleaseName: "one", Namespace: "default"},
		},
		Status: bootstrapv1.ArtifactInstallationStatus{OperationID: "operation-one"},
	}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-default-profile-slot", Namespace: "default", Labels: map[string]string{"w7.cc/bootstrap-slot": "true"}},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("operation-one")},
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(installation, lease).Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer, slots: newLeaseSlots(k8sClient, "default")}

	if _, err := reconciler.reconcileDeletion(context.Background(), installation); err != nil {
		t.Fatal(err)
	}
	if installer.uninstallCalls != 1 {
		t.Fatalf("uninstall calls = %d, want 1", installer.uninstallCalls)
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	for _, finalizer := range updated.Finalizers {
		if finalizer == bootstrapv1.ArtifactFinalizer {
			t.Fatalf("artifact finalizer was not removed: %v", updated.Finalizers)
		}
	}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: lease.Name, Namespace: lease.Namespace}, lease); err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != "" {
		t.Fatalf("lease holder = %v, want released during deletion", lease.Spec.HolderIdentity)
	}
}

func TestArtifactReconcilerInstallsUnspecifiedVersionWithoutPreResolving(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1", Artifacts: []bootstrapv1.BootstrapArtifact{artifact}},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile-one", Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(installation, profile).WithObjects(profile, installation).Build()
	reconciler := &ArtifactReconciler{
		Client: k8sClient, Scheme: scheme, installer: installer,
		slots: newLeaseSlots(k8sClient, "default"),
	}
	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseInstalling {
		t.Fatalf("phase = %q, want %q; status=%#v", updated.Status.Phase, bootstrapv1.BootstrapPhaseInstalling, updated.Status)
	}
	if updated.Status.ObservedProfileRevision != profile.Spec.Revision {
		t.Fatalf("observed revision = %q", updated.Status.ObservedProfileRevision)
	}
	if installer.installCalls != 1 {
		t.Fatalf("install calls=%d, want 1", installer.installCalls)
	}
	if installer.installTimeout <= 0 || installer.installTimeout > defaultArtifactTimeout {
		t.Fatalf("install timeout = %s, want a positive timeout no greater than %s", installer.installTimeout, defaultArtifactTimeout)
	}
}

func TestArtifactReconcilerHoldsLeaseUntilAppGroupReady(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision:  "1.0.0-1",
			Strategy:  bootstrapv1.BootstrapStrategy{MaxConcurrent: 1},
			Artifacts: []bootstrapv1.BootstrapArtifact{artifact},
		},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{
		Client: k8sClient, Scheme: scheme, installer: installer,
		slots: newLeaseSlots(k8sClient, "default"),
	}
	request := ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}}

	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	lease := &coordinationv1.Lease{}
	leaseKey := types.NamespacedName{Name: slotLeaseName(profile.Name, 0), Namespace: "default"}
	if err := k8sClient.Get(context.Background(), leaseKey, lease); err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		t.Fatal("expected install operation to hold the concurrency lease")
	}

	installer.installed = &installedArtifact{
		Name: "one", Namespace: "default", Identifie: "one", State: installedArtifactInstalling,
	}
	if result, err := reconciler.Reconcile(context.Background(), request); err != nil || result.RequeueAfter == 0 {
		t.Fatalf("poll installing AppGroup = %#v, %v", result, err)
	}
	if err := k8sClient.Get(context.Background(), leaseKey, lease); err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		t.Fatal("lease was released before AppGroup became ready")
	}

	installer.installed.State = installedArtifactReady
	if _, err := reconciler.Reconcile(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Get(context.Background(), leaseKey, lease); err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != "" {
		t.Fatalf("lease holder = %v, want released after AppGroup ready", lease.Spec.HolderIdentity)
	}
}

func TestArtifactReconcilerCancelsInstallAtArtifactTimeout(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision:  "1.0.0-1",
			Strategy:  bootstrapv1.BootstrapStrategy{MaxConcurrent: 1, TimeoutPerArtifact: metav1.Duration{Duration: 20 * time.Millisecond}},
			Artifacts: []bootstrapv1.BootstrapArtifact{artifact},
		},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
	}
	installer := &fakeArtifactInstaller{waitForCancel: true}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{
		Client: k8sClient, Scheme: scheme, installer: installer,
		slots: newLeaseSlots(k8sClient, "default"),
	}

	started := time.Now()
	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("install did not stop promptly at timeout")
	}
	if result.RequeueAfter == 0 {
		t.Fatal("timed out install was not scheduled for retry")
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhasePending || updated.Status.RetryCount != 1 {
		t.Fatalf("status after timeout = %#v, want Pending with one retry", updated.Status)
	}
	lease := &coordinationv1.Lease{}
	leaseKey := types.NamespacedName{Name: slotLeaseName(profile.Name, 0), Namespace: "default"}
	if err := k8sClient.Get(context.Background(), leaseKey, lease); err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != "" {
		t.Fatalf("lease holder = %v, want released after timeout", lease.Spec.HolderIdentity)
	}
}

func TestArtifactReconcilerWaitsForExistingAppGroupToBecomeReady(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default", Version: "2.0.0",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1", Artifacts: []bootstrapv1.BootstrapArtifact{artifact}},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: profile.Spec.Revision,
			Phase:                   bootstrapv1.BootstrapPhaseFailed,
		},
	}
	group := &appgroupv1.AppGroup{
		ObjectMeta: metav1.ObjectMeta{Name: artifact.ReleaseName, Namespace: artifact.Namespace},
		Spec: appgroupv1.AppGroupSpec{
			Identifie: artifact.Identifie,
			Version:   "1.0.0",
		},
		Status: appgroupv1.AppGroupStatus{Ready: false, DeployStatus: appgroupv1.StatusFailed},
	}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: group.Name, Namespace: group.Namespace,
		Identifie: group.Spec.Identifie, Version: group.Spec.Version, State: installedArtifactInstalling,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}}); err != nil {
		t.Fatal(err)
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseInstalling {
		t.Fatalf("phase = %q, want %q while AppGroup is installing; status=%#v", updated.Status.Phase, bootstrapv1.BootstrapPhaseInstalling, updated.Status)
	}
	installer.installed.State = installedArtifactReady
	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseReady {
		t.Fatalf("phase = %q, want %q after AppGroup becomes ready; status=%#v", updated.Status.Phase, bootstrapv1.BootstrapPhaseReady, updated.Status)
	}
	if updated.Status.InstalledVersion != group.Spec.Version {
		t.Fatalf("installed version = %q, want existing AppGroup version %q", updated.Status.InstalledVersion, group.Spec.Version)
	}
	if installer.lookupCalls != 2 {
		t.Fatalf("lookup calls=%d, want 2", installer.lookupCalls)
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls=%d, want no artifact operation", installer.installCalls)
	}
}

func TestArtifactReconcilerTimesOutExistingAppGroup(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision:  "1.0.0-1",
			Strategy:  bootstrapv1.BootstrapStrategy{MaxRetries: 1, TimeoutPerArtifact: metav1.Duration{Duration: time.Second}},
			Artifacts: []bootstrapv1.BootstrapArtifact{artifact},
		},
	}
	startedAt := metav1.NewTime(time.Now().Add(-2 * time.Second))
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: profile.Spec.Revision,
			Phase:                   bootstrapv1.BootstrapPhaseInstalling,
			StartedAt:               &startedAt,
		},
	}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "one", Namespace: "default", Identifie: "one", State: installedArtifactInstalling,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("failed existing AppGroup should remain observable")
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseFailed || updated.Status.RetryCount != 1 {
		t.Fatalf("status after existing AppGroup timeout = %#v", updated.Status)
	}
}

func TestArtifactReconcilerWaitsForInProgressOperationWithoutAppGroupMetadata(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default", Version: "1.2.3",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1", Artifacts: []bootstrapv1.BootstrapArtifact{artifact}},
	}
	startedAt := metav1.Now()
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: profile.Spec.Revision,
			Phase:                   bootstrapv1.BootstrapPhaseInstalling,
			OperationID:             "operation-one",
			StartedAt:               &startedAt,
		},
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{
		Client: k8sClient, Scheme: scheme, installer: installer,
		slots: newLeaseSlots(k8sClient, "default"),
	}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected in-progress operation to be polled")
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0 while waiting for the existing operation", installer.installCalls)
	}
}

func TestArtifactReconcilerBlocksWhileApplicationDeleting(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1", Artifacts: []bootstrapv1.BootstrapArtifact{artifact}},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: profile.Spec.Revision,
			Phase:                   bootstrapv1.BootstrapPhaseReady,
		},
	}
	installer := &fakeArtifactInstaller{installed: &installedArtifact{
		Name: "one", Namespace: "default", Identifie: "one", State: installedArtifactDeleting,
	}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter == 0 {
		t.Fatal("expected deleting application to be requeued")
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseBlocked || installer.installCalls != 0 {
		t.Fatalf("status=%#v installCalls=%d", updated.Status, installer.installCalls)
	}
}

func TestArtifactReconcilerDoesNotReinstallDeletedReadyApplication(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	artifact := bootstrapv1.BootstrapArtifact{
		Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default",
	}
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec:       bootstrapv1.BootstrapProfileSpec{Revision: "1.0.0-1", Artifacts: []bootstrapv1.BootstrapArtifact{artifact}},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName(profile.Name, artifact.Name), Finalizers: []string{bootstrapv1.ArtifactFinalizer}},
		Spec:       effectiveArtifact(profile, artifact),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: profile.Spec.Revision,
			Phase:                   bootstrapv1.BootstrapPhaseReady,
		},
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer}

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: installation.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Requeue || result.RequeueAfter != 0 {
		t.Fatalf("deleted ready application must not be requeued: %#v", result)
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls = %d, want 0", installer.installCalls)
	}
}

func TestArtifactOwnership(t *testing.T) {
	installation := &bootstrapv1.ArtifactInstallation{Spec: bootstrapv1.ArtifactInstallationSpec{
		ProfileRef: bootstrapv1.BootstrapProfileReference{UID: "profile-uid"},
		Artifact:   bootstrapv1.ArtifactReference{Name: "one"},
		InstallOptions: bootstrapv1.BootstrapInstallOptions{Annotations: map[string]string{
			"w7.cc/deny-delete":                 "true",
			bootstrapv1.AnnotationArtifactOwner: "untrusted-value",
		}},
	}}
	annotations := artifactInstallAnnotations(installation)
	if annotations["w7.cc/deny-delete"] != "true" {
		t.Fatalf("custom annotation was not preserved: %v", annotations)
	}
	if annotations[bootstrapv1.AnnotationArtifactOwner] != "profile-uid/one" {
		t.Fatalf("owner annotation was not enforced: %v", annotations)
	}
	if !isArtifactOwner(annotations, installation) {
		t.Fatal("expected matching owner annotation")
	}
	annotations[bootstrapv1.AnnotationArtifactOwner] = "another-profile/one"
	if isArtifactOwner(annotations, installation) {
		t.Fatal("unexpected ownership for adopted application")
	}
}

func TestProfileStatusDoesNotReusePreviousRevisionTerminalState(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	profile := &bootstrapv1.BootstrapProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile", UID: types.UID("profile-uid")},
		Spec: bootstrapv1.BootstrapProfileSpec{
			Revision: "2.0.0-1",
			Artifacts: []bootstrapv1.BootstrapArtifact{
				{Name: "one", Identifie: "one", Source: "https://zpk.w7.cc/info/one", ReleaseName: "one", Namespace: "default"},
			},
		},
	}
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{
			Name:   artifactInstallationName(profile.Name, "one"),
			Labels: map[string]string{bootstrapv1.LabelProfile: profile.Name, bootstrapv1.LabelArtifact: "one"},
		},
		Spec: effectiveArtifact(profile, profile.Spec.Artifacts[0]),
		Status: bootstrapv1.ArtifactInstallationStatus{
			ObservedProfileRevision: "1.0.0-1",
			Phase:                   bootstrapv1.BootstrapPhaseReady,
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(profile, installation).
		WithObjects(profile, installation).
		Build()
	reconciler := &ProfileReconciler{Client: k8sClient, Scheme: scheme}

	if err := reconciler.updateProfileStatus(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	updated := &bootstrapv1.BootstrapProfile{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: profile.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != bootstrapv1.ProfilePhaseProgressing {
		t.Fatalf("profile phase = %q, want %q", updated.Status.Phase, bootstrapv1.ProfilePhaseProgressing)
	}
	if updated.Status.Summary.Ready != 0 || updated.Status.Summary.Progressing != 1 {
		t.Fatalf("profile summary = %#v, want one progressing artifact", updated.Status.Summary)
	}
}

func TestDependenciesReadyHonorsFailurePolicy(t *testing.T) {
	tests := []struct {
		name          string
		failurePolicy bootstrapv1.FailurePolicy
		phase         bootstrapv1.BootstrapPhase
		wantReady     bool
	}{
		{name: "stop blocks failed dependency", failurePolicy: bootstrapv1.FailurePolicyStop, phase: bootstrapv1.BootstrapPhaseFailed},
		{name: "continue allows failed dependency", failurePolicy: bootstrapv1.FailurePolicyContinue, phase: bootstrapv1.BootstrapPhaseFailed, wantReady: true},
		{name: "ready dependency proceeds", failurePolicy: bootstrapv1.FailurePolicyStop, phase: bootstrapv1.BootstrapPhaseReady, wantReady: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheme := bootstrapTestScheme(t)
			dependency := &bootstrapv1.ArtifactInstallation{
				ObjectMeta: metav1.ObjectMeta{Name: artifactInstallationName("default-profile", "dependency")},
				Spec: bootstrapv1.ArtifactInstallationSpec{
					ProfileRevision: "1.0.0-1",
					FailurePolicy:   test.failurePolicy,
				},
				Status: bootstrapv1.ArtifactInstallationStatus{Phase: test.phase},
			}
			installation := &bootstrapv1.ArtifactInstallation{
				Spec: bootstrapv1.ArtifactInstallationSpec{
					ProfileRef:      bootstrapv1.BootstrapProfileReference{Name: "default-profile"},
					ProfileRevision: "1.0.0-1",
					DependsOn:       []string{"dependency"},
				},
			}
			k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dependency).Build()
			reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme}

			ready, _, err := reconciler.dependenciesReady(context.Background(), installation)
			if err != nil {
				t.Fatal(err)
			}
			if ready != test.wantReady {
				t.Fatalf("dependenciesReady() = %v, want %v", ready, test.wantReady)
			}
		})
	}
}

func TestUpdateStatusRecordsObservedRevision(t *testing.T) {
	scheme := bootstrapTestScheme(t)
	installation := &bootstrapv1.ArtifactInstallation{
		ObjectMeta: metav1.ObjectMeta{Name: "default-profile-one"},
		Spec:       bootstrapv1.ArtifactInstallationSpec{ProfileRevision: "2.0.0-1"},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(installation).
		WithObjects(installation).
		Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme}

	if err := reconciler.updateStatus(context.Background(), installation, bootstrapv1.BootstrapPhaseBlocked, "waiting", nil, false); err != nil {
		t.Fatal(err)
	}
	updated := &bootstrapv1.ArtifactInstallation{}
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Name: installation.Name}, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.ObservedProfileRevision != installation.Spec.ProfileRevision {
		t.Fatalf("observed revision = %q, want %q", updated.Status.ObservedProfileRevision, installation.Spec.ProfileRevision)
	}
}
