package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var errLeaseBusy = errors.New("bootstrap lease slot is busy")

type leaseSlots struct {
	client    client.Client
	namespace string
}

func newLeaseSlots(k8sClient client.Client, namespace string) *leaseSlots {
	if namespace == "" {
		namespace = "default"
	}
	return &leaseSlots{client: k8sClient, namespace: namespace}
}

func (s *leaseSlots) acquire(ctx context.Context, profileName, holder string, limit int32, timeout time.Duration) (bool, error) {
	for index := int32(0); index < limit; index++ {
		name := slotLeaseName(profileName, index)
		lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: s.namespace}}
		_, err := controllerutil.CreateOrUpdate(ctx, s.client, lease, func() error {
			if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != holder && !leaseAvailable(lease, time.Now()) {
				return errLeaseBusy
			}
			now := metav1.MicroTime{Time: time.Now()}
			if lease.Labels == nil {
				lease.Labels = map[string]string{}
			}
			lease.Labels["w7.cc/bootstrap-slot"] = "true"
			seconds := max(int32(timeout.Seconds()), 60)
			if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != holder {
				lease.Spec.AcquireTime = &now
			}
			lease.Spec.HolderIdentity = ptr.To(holder)
			lease.Spec.LeaseDurationSeconds = ptr.To(seconds)
			lease.Spec.RenewTime = &now
			return nil
		})
		if err == nil {
			return true, nil
		}
		if errors.Is(err, errLeaseBusy) || apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
			continue
		}
		return false, fmt.Errorf("占用 Bootstrap Lease %q: %w", name, err)
	}
	return false, nil
}

func (s *leaseSlots) release(ctx context.Context, holder string) error {
	leases := &coordinationv1.LeaseList{}
	if err := s.client.List(ctx, leases, client.InNamespace(s.namespace), client.MatchingLabels{"w7.cc/bootstrap-slot": "true"}); err != nil {
		return err
	}
	for index := range leases.Items {
		lease := &leases.Items[index]
		if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != holder {
			continue
		}
		base := lease.DeepCopy()
		lease.Spec.HolderIdentity = ptr.To("")
		lease.Spec.AcquireTime = nil
		lease.Spec.RenewTime = nil
		if err := s.client.Patch(ctx, lease, client.MergeFrom(base)); err != nil && !apierrors.IsConflict(err) {
			return err
		}
	}
	return nil
}

func leaseAvailable(lease *coordinationv1.Lease, now time.Time) bool {
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" || lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	return lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second).Before(now)
}

func slotLeaseName(profileName string, index int32) string {
	base := strings.Trim(profileName, "-.")
	if len(base) > 40 {
		base = base[:40]
	}
	sum := sha256.Sum256([]byte(profileName))
	return fmt.Sprintf("bootstrap-%s-%s-%d", base, hex.EncodeToString(sum[:4]), index)
}
