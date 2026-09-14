// Package coordination provides reusable Kubernetes Lease based coordination primitives.
package coordination

import (
	"context"
	"fmt"
	"math"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LeaseManager implements atomic acquire/renew/release operations. Its Reader
// should bypass the controller cache (for example manager.GetAPIReader()).
type LeaseManager struct {
	reader client.Reader
	writer client.Client
}

func NewLeaseManager(reader client.Reader, writer client.Client) *LeaseManager {
	return &LeaseManager{reader: reader, writer: writer}
}

// TryAcquire creates a Lease, renews it for the same holder, or takes over an
// expired Lease. It returns false when another live holder owns the Lease.
func (m *LeaseManager) TryAcquire(ctx context.Context, key client.ObjectKey, holder string, duration time.Duration, labels map[string]string) (bool, error) {
	if key.Name == "" || key.Namespace == "" {
		return false, fmt.Errorf("Lease namespace 和 name 不能为空")
	}
	if holder == "" {
		return false, fmt.Errorf("Lease holder 不能为空")
	}
	durationSeconds, err := durationToSeconds(duration)
	if err != nil {
		return false, err
	}

	acquired := false
	err = retry.OnError(retry.DefaultRetry, func(err error) bool {
		return apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err)
	}, func() error {
		lease := &coordinationv1.Lease{}
		err := m.reader.Get(ctx, key, lease)
		if apierrors.IsNotFound(err) {
			now := metav1.MicroTime{Time: time.Now()}
			lease = &coordinationv1.Lease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      key.Name,
					Namespace: key.Namespace,
					Labels:    cloneLabels(labels),
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity:       ptr.To(holder),
					LeaseDurationSeconds: ptr.To(durationSeconds),
					AcquireTime:          &now,
					RenewTime:            &now,
				},
			}
			if err := m.writer.Create(ctx, lease); err != nil {
				return err
			}
			acquired = true
			return nil
		}
		if err != nil {
			return err
		}

		now := time.Now()
		if lease.Spec.HolderIdentity != nil &&
			*lease.Spec.HolderIdentity != holder &&
			!leaseAvailable(lease, now) {
			acquired = false
			return nil
		}

		base := lease.DeepCopy()
		if lease.Labels == nil {
			lease.Labels = map[string]string{}
		}
		for name, value := range labels {
			lease.Labels[name] = value
		}
		microNow := metav1.MicroTime{Time: now}
		if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != holder {
			lease.Spec.AcquireTime = &microNow
			lease.Spec.LeaseTransitions = ptr.To(ptr.Deref(lease.Spec.LeaseTransitions, 0) + 1)
		}
		lease.Spec.HolderIdentity = ptr.To(holder)
		lease.Spec.LeaseDurationSeconds = ptr.To(durationSeconds)
		lease.Spec.RenewTime = &microNow
		if err := m.writer.Patch(ctx, lease, client.MergeFrom(base)); err != nil {
			return err
		}
		acquired = true
		return nil
	})
	return acquired, err
}

// Release clears a Lease only when holder still owns it. A holder that lost the
// Lease can therefore never release a newer owner's lock.
func (m *LeaseManager) Release(ctx context.Context, key client.ObjectKey, holder string) error {
	if holder == "" {
		return fmt.Errorf("Lease holder 不能为空")
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		lease := &coordinationv1.Lease{}
		if err := m.reader.Get(ctx, key, lease); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != holder {
			return nil
		}
		base := lease.DeepCopy()
		lease.Spec.HolderIdentity = ptr.To("")
		lease.Spec.AcquireTime = nil
		lease.Spec.RenewTime = nil
		return m.writer.Patch(ctx, lease, client.MergeFrom(base))
	})
}

func leaseAvailable(lease *coordinationv1.Lease, now time.Time) bool {
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" || lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	return !lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second).After(now)
}

func durationToSeconds(duration time.Duration) (int32, error) {
	if duration <= 0 {
		return 0, fmt.Errorf("Lease duration 必须大于 0")
	}
	seconds := int64(math.Ceil(duration.Seconds()))
	if seconds > math.MaxInt32 {
		return math.MaxInt32, nil
	}
	return int32(seconds), nil
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for name, value := range labels {
		result[name] = value
	}
	return result
}
