package coordination

import (
	"context"
	"errors"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type patchConflictClient struct {
	client.Client
	remainingConflicts int
	patchCalls         int
}

func (c *patchConflictClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if _, ok := obj.(*coordinationv1.Lease); ok {
		c.patchCalls++
		if c.remainingConflicts > 0 {
			c.remainingConflicts--
			return apierrors.NewConflict(
				schema.GroupResource{Group: coordinationv1.GroupName, Resource: "leases"},
				obj.GetName(),
				errors.New("injected conflict"),
			)
		}
	}
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func TestLeaseManagerAcquireRenewAndRejectOtherHolder(t *testing.T) {
	ctx := context.Background()
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	manager := NewLeaseManager(k8sClient, k8sClient)
	key := client.ObjectKey{Name: "resource-lock", Namespace: "default"}

	if acquired, err := manager.TryAcquire(ctx, key, "pod-a/uid-a", time.Minute, map[string]string{"purpose": "test"}); err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	if acquired, err := manager.TryAcquire(ctx, key, "pod-a/uid-a", time.Minute, nil); err != nil || !acquired {
		t.Fatalf("renew = %v, %v", acquired, err)
	}
	if acquired, err := manager.TryAcquire(ctx, key, "pod-b/uid-b", time.Minute, nil); err != nil || acquired {
		t.Fatalf("other holder acquire = %v, %v; want busy", acquired, err)
	}
}

func TestLeaseManagerExpiredTakeoverAndSafeRelease(t *testing.T) {
	ctx := context.Background()
	renewedAt := metav1.NewMicroTime(time.Now().Add(-2 * time.Minute))
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "resource-lock", Namespace: "default"},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To("pod-a/uid-a"),
			RenewTime:            &renewedAt,
			LeaseDurationSeconds: ptr.To[int32](60),
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(lease).Build()
	manager := NewLeaseManager(k8sClient, k8sClient)
	key := client.ObjectKeyFromObject(lease)

	if acquired, err := manager.TryAcquire(ctx, key, "pod-b/uid-b", time.Minute, nil); err != nil || !acquired {
		t.Fatalf("expired takeover = %v, %v", acquired, err)
	}
	if err := manager.Release(ctx, key, "pod-a/uid-a"); err != nil {
		t.Fatal(err)
	}
	updated := &coordinationv1.Lease{}
	if err := k8sClient.Get(ctx, key, updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.HolderIdentity == nil || *updated.Spec.HolderIdentity != "pod-b/uid-b" {
		t.Fatalf("old holder released new owner: %v", updated.Spec.HolderIdentity)
	}
	if updated.Spec.LeaseTransitions == nil || *updated.Spec.LeaseTransitions != 1 {
		t.Fatalf("lease transitions = %v, want 1", updated.Spec.LeaseTransitions)
	}
}

func TestLeaseManagerReleaseRetriesConflict(t *testing.T) {
	ctx := context.Background()
	lease := activeLease("resource-lock", "operation-one", nil)
	baseClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(lease).Build()
	conflictClient := &patchConflictClient{Client: baseClient, remainingConflicts: 1}
	manager := NewLeaseManager(conflictClient, conflictClient)

	if err := manager.Release(ctx, client.ObjectKeyFromObject(lease), "operation-one"); err != nil {
		t.Fatal(err)
	}
	if conflictClient.patchCalls != 2 {
		t.Fatalf("patch calls = %d, want conflict plus retry", conflictClient.patchCalls)
	}
}

func TestLeaseAvailableAtExpirationBoundary(t *testing.T) {
	expiresAt := time.Unix(100, 0)
	renewedAt := metav1.NewMicroTime(expiresAt.Add(-time.Minute))
	lease := &coordinationv1.Lease{Spec: coordinationv1.LeaseSpec{
		HolderIdentity:       ptr.To("operation-one"),
		RenewTime:            &renewedAt,
		LeaseDurationSeconds: ptr.To[int32](60),
	}}
	if !leaseAvailable(lease, expiresAt) {
		t.Fatal("lease should be available exactly at its expiration time")
	}
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func activeLease(name, holder string, labels map[string]string) *coordinationv1.Lease {
	now := metav1.NowMicro()
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: labels},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To(holder),
			AcquireTime:          &now,
			RenewTime:            &now,
			LeaseDurationSeconds: ptr.To[int32](60),
		},
	}
}
