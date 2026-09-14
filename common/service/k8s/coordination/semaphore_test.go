package coordination

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSemaphoreRetriesSameSlotAfterConflict(t *testing.T) {
	ctx := context.Background()
	selector := map[string]string{"w7.cc/test-slot": "true"}
	lease := activeLease(SemaphoreSlotName("test", "profile", 0), "operation-one", selector)
	baseClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(lease).Build()
	conflictClient := &patchConflictClient{Client: baseClient, remainingConflicts: 1}
	semaphore := NewSemaphore(conflictClient, conflictClient, SemaphoreOptions{
		Namespace: "default", NamePrefix: "test", Selector: selector, MinimumDuration: time.Minute,
	})

	acquired, err := semaphore.Acquire(ctx, "profile", "operation-one", 2, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("acquire after conflict = %v, %v", acquired, err)
	}
	if conflictClient.patchCalls != 2 {
		t.Fatalf("patch calls = %d, want conflict plus same-slot retry", conflictClient.patchCalls)
	}

	secondSlot := &coordinationv1.Lease{}
	err = baseClient.Get(ctx, client.ObjectKey{Name: semaphore.SlotName("profile", 1), Namespace: "default"}, secondSlot)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("same operation unexpectedly occupied a second slot: %v", err)
	}
}

func TestSemaphoreLimitsAndReleasesLogicalOperations(t *testing.T) {
	ctx := context.Background()
	selector := map[string]string{"w7.cc/test-slot": "true"}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	semaphore := NewSemaphore(k8sClient, k8sClient, SemaphoreOptions{
		Namespace: "default", NamePrefix: "test", Selector: selector, MinimumDuration: time.Minute,
	})

	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-one", 1, time.Minute); err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-two", 1, time.Minute); err != nil || acquired {
		t.Fatalf("second acquire = %v, %v; want busy", acquired, err)
	}
	if err := semaphore.Release(ctx, "operation-one"); err != nil {
		t.Fatal(err)
	}
	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-two", 1, time.Minute); err != nil || !acquired {
		t.Fatalf("acquire after release = %v, %v", acquired, err)
	}
}

func TestSemaphoreRenewsExistingHigherSlotBeforeTakingFreeLowerSlot(t *testing.T) {
	ctx := context.Background()
	selector := map[string]string{"w7.cc/test-slot": "true"}
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	semaphore := NewSemaphore(k8sClient, k8sClient, SemaphoreOptions{
		Namespace: "default", NamePrefix: "test", Selector: selector, MinimumDuration: time.Minute,
	})

	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-one", 2, time.Minute); err != nil || !acquired {
		t.Fatalf("operation one acquire = %v, %v", acquired, err)
	}
	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-two", 2, time.Minute); err != nil || !acquired {
		t.Fatalf("operation two acquire = %v, %v", acquired, err)
	}
	if err := semaphore.Release(ctx, "operation-one"); err != nil {
		t.Fatal(err)
	}
	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-two", 2, time.Minute); err != nil || !acquired {
		t.Fatalf("operation two renew = %v, %v", acquired, err)
	}

	lower := &coordinationv1.Lease{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: semaphore.SlotName("profile", 0), Namespace: "default"}, lower); err != nil {
		t.Fatal(err)
	}
	if lower.Spec.HolderIdentity == nil || *lower.Spec.HolderIdentity != "" {
		t.Fatalf("lower slot holder = %v, want released", lower.Spec.HolderIdentity)
	}
	higher := &coordinationv1.Lease{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: semaphore.SlotName("profile", 1), Namespace: "default"}, higher); err != nil {
		t.Fatal(err)
	}
	if higher.Spec.HolderIdentity == nil || *higher.Spec.HolderIdentity != "operation-two" {
		t.Fatalf("higher slot holder = %v, want operation-two", higher.Spec.HolderIdentity)
	}
}

func TestSemaphoreDefaultSelectorDoesNotReleaseUnrelatedLease(t *testing.T) {
	ctx := context.Background()
	unrelated := activeLease("unrelated", "operation-one", nil)
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(unrelated).Build()
	semaphore := NewSemaphore(k8sClient, k8sClient, SemaphoreOptions{
		Namespace: "default", NamePrefix: "test", MinimumDuration: time.Minute,
	})

	if acquired, err := semaphore.Acquire(ctx, "profile", "operation-one", 1, time.Minute); err != nil || !acquired {
		t.Fatalf("acquire = %v, %v", acquired, err)
	}
	if err := semaphore.Release(ctx, "operation-one"); err != nil {
		t.Fatal(err)
	}
	updated := &coordinationv1.Lease{}
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(unrelated), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Spec.HolderIdentity == nil || *updated.Spec.HolderIdentity != "operation-one" {
		t.Fatalf("unrelated lease holder changed: %v", updated.Spec.HolderIdentity)
	}
}

func TestSemaphoreSlotNameSanitizesArbitraryGroup(t *testing.T) {
	name := SemaphoreSlotName("Resource Mutex", "apps/v1:Default/My_App", 3)
	if errors := validation.IsDNS1123Subdomain(name); len(errors) > 0 {
		t.Fatalf("slot name %q is invalid: %v", name, errors)
	}
	if name != SemaphoreSlotName("Resource Mutex", "apps/v1:Default/My_App", 3) {
		t.Fatal("slot naming must be deterministic")
	}
}
