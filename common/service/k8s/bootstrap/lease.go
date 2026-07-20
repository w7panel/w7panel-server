package bootstrap

import (
	"context"
	"time"

	leasecoordination "github.com/w7panel/w7panel/common/service/k8s/coordination"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const bootstrapSlotLabel = "w7.cc/bootstrap-slot"

type leaseSlots struct {
	semaphore *leasecoordination.Semaphore
}

func newLeaseSlots(k8sClient client.Client, namespace string) *leaseSlots {
	return newLeaseSlotsWithReader(k8sClient, k8sClient, namespace)
}

func newLeaseSlotsWithReader(reader client.Reader, writer client.Client, namespace string) *leaseSlots {
	return &leaseSlots{semaphore: leasecoordination.NewSemaphore(reader, writer, leasecoordination.SemaphoreOptions{
		Namespace:       namespace,
		NamePrefix:      "bootstrap",
		Selector:        map[string]string{bootstrapSlotLabel: "true"},
		MinimumDuration: time.Minute,
	})}
}

func (s *leaseSlots) acquire(ctx context.Context, profileName, holder string, limit int32, timeout time.Duration) (bool, error) {
	return s.semaphore.Acquire(ctx, profileName, holder, limit, timeout)
}

func (s *leaseSlots) release(ctx context.Context, holder string) error {
	return s.semaphore.Release(ctx, holder)
}

func slotLeaseName(profileName string, index int32) string {
	return leasecoordination.SemaphoreSlotName("bootstrap", profileName, index)
}
