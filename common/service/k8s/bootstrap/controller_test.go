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
}

func (f *fakeArtifactInstaller) Lookup(context.Context, *bootstrapv1.ArtifactInstallation) (*installedArtifact, error) {
	f.lookupCalls++
	return f.installed, f.lookupErr
}

func (f *fakeArtifactInstaller) InstallOrUpgrade(context.Context, *bootstrapv1.ArtifactInstallation) error {
	f.installCalls++
	return f.err
}

func (f *fakeArtifactInstaller) Uninstall(context.Context, string, string) error {
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
	}
	installer := &fakeArtifactInstaller{}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(installation).Build()
	reconciler := &ArtifactReconciler{Client: k8sClient, Scheme: scheme, installer: installer}

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
}

func TestArtifactReconcilerTreatsExistingAppGroupAsInstalledRegardlessOfVersionOrStatus(t *testing.T) {
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
			Phase:                   bootstrapv1.BootstrapPhaseAheadOfProfile,
			ResolvedVersion:         artifact.Version,
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
		Identifie: group.Spec.Identifie, Version: group.Spec.Version,
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
	if updated.Status.Phase != bootstrapv1.BootstrapPhaseReady {
		t.Fatalf("phase = %q, want %q; status=%#v", updated.Status.Phase, bootstrapv1.BootstrapPhaseReady, updated.Status)
	}
	if updated.Status.InstalledVersion != group.Spec.Version {
		t.Fatalf("installed version = %q, want existing AppGroup version %q", updated.Status.InstalledVersion, group.Spec.Version)
	}
	if installer.lookupCalls != 1 {
		t.Fatalf("lookup calls=%d, want 1", installer.lookupCalls)
	}
	if installer.installCalls != 0 {
		t.Fatalf("install calls=%d, want no artifact operation", installer.installCalls)
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
			ResolvedVersion:         artifact.Version,
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
