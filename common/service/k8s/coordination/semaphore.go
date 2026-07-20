package coordination

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultSemaphoreLabel = "w7.cc/semaphore"

// SemaphoreOptions configures the namespace, naming and discovery labels for
// one independent group of distributed semaphores.
type SemaphoreOptions struct {
	Namespace       string
	NamePrefix      string
	Selector        map[string]string
	MinimumDuration time.Duration
}

// Semaphore is a durable, distributed set of Lease-backed concurrency slots.
// Holder identifies a logical operation, so another controller replica can
// continue renewing the same operation after failover. Holders must be unique
// across every group that shares the same selector because Release finds slots
// by selector and holder.
type Semaphore struct {
	leases          *LeaseManager
	namespace       string
	namePrefix      string
	selector        map[string]string
	minimumDuration time.Duration
}

func NewSemaphore(reader client.Reader, writer client.Client, options SemaphoreOptions) *Semaphore {
	namespace := options.Namespace
	if namespace == "" {
		namespace = "default"
	}
	prefix := sanitizeNameSegment(options.NamePrefix, "semaphore")
	minimumDuration := options.MinimumDuration
	if minimumDuration <= 0 {
		minimumDuration = time.Second
	}
	selector := cloneLabels(options.Selector)
	if len(selector) == 0 {
		labelValue := prefix
		if len(labelValue) > 63 {
			labelValue = labelValue[:63]
		}
		selector = map[string]string{defaultSemaphoreLabel: labelValue}
	}
	return &Semaphore{
		leases:          NewLeaseManager(reader, writer),
		namespace:       namespace,
		namePrefix:      prefix,
		selector:        selector,
		minimumDuration: minimumDuration,
	}
}

func (s *Semaphore) Acquire(ctx context.Context, group, holder string, limit int32, duration time.Duration) (bool, error) {
	if group == "" {
		return false, fmt.Errorf("Semaphore group 不能为空")
	}
	if holder == "" {
		return false, fmt.Errorf("Semaphore holder 不能为空")
	}
	if limit <= 0 {
		return false, nil
	}
	if duration < s.minimumDuration {
		duration = s.minimumDuration
	}

	// Renew the holder's existing slot before looking for a free lower-indexed
	// slot. Otherwise a holder on slot 1 could also occupy slot 0 after it is
	// released, consuming multiple permits for one logical operation.
	existingKeys, err := s.holderKeys(ctx, holder)
	if err != nil {
		return false, err
	}
	if len(existingKeys) > 0 {
		acquired, err := s.leases.TryAcquire(ctx, existingKeys[0], holder, duration, s.selector)
		if err != nil {
			return false, fmt.Errorf("续租 Semaphore Lease %q: %w", existingKeys[0].Name, err)
		}
		if acquired {
			for _, duplicateKey := range existingKeys[1:] {
				if err := s.leases.Release(ctx, duplicateKey, holder); err != nil {
					return false, fmt.Errorf("清理重复 Semaphore Lease %q: %w", duplicateKey.Name, err)
				}
			}
			return true, nil
		}
	}

	for index := int32(0); index < limit; index++ {
		key := client.ObjectKey{Name: s.SlotName(group, index), Namespace: s.namespace}
		acquired, err := s.leases.TryAcquire(ctx, key, holder, duration, s.selector)
		if err != nil {
			return false, fmt.Errorf("占用 Semaphore Lease %q: %w", key.Name, err)
		}
		if acquired {
			return true, nil
		}
	}
	return false, nil
}

func (s *Semaphore) holderKeys(ctx context.Context, holder string) ([]client.ObjectKey, error) {
	leases := &coordinationv1.LeaseList{}
	if err := s.leases.reader.List(ctx, leases, client.InNamespace(s.namespace), client.MatchingLabels(s.selector)); err != nil {
		return nil, err
	}
	keys := make([]client.ObjectKey, 0, 1)
	for index := range leases.Items {
		lease := &leases.Items[index]
		if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity == holder {
			keys = append(keys, client.ObjectKeyFromObject(lease))
		}
	}
	sort.Slice(keys, func(left, right int) bool {
		return keys[left].Name < keys[right].Name
	})
	return keys, nil
}

func (s *Semaphore) Release(ctx context.Context, holder string) error {
	if holder == "" {
		return fmt.Errorf("Semaphore holder 不能为空")
	}
	keys, err := s.holderKeys(ctx, holder)
	if err != nil {
		return fmt.Errorf("查询 Semaphore Holder Lease: %w", err)
	}
	for _, key := range keys {
		if err := s.leases.Release(ctx, key, holder); err != nil {
			return fmt.Errorf("释放 Semaphore Lease %q: %w", key.Name, err)
		}
	}
	return nil
}

func (s *Semaphore) SlotName(group string, index int32) string {
	return SemaphoreSlotName(s.namePrefix, group, index)
}

func SemaphoreSlotName(prefix, group string, index int32) string {
	prefix = sanitizeNameSegment(prefix, "semaphore")
	if len(prefix) > 40 {
		prefix = strings.TrimRight(prefix[:40], "-.")
	}
	base := sanitizeNameSegment(group, "group")
	if len(base) > 40 {
		base = strings.TrimRight(base[:40], "-.")
	}
	sum := sha256.Sum256([]byte(group))
	return fmt.Sprintf("%s-%s-%s-%d", prefix, base, hex.EncodeToString(sum[:4]), index)
}

func sanitizeNameSegment(value, fallback string) string {
	value = strings.ToLower(value)
	var result strings.Builder
	result.Grow(len(value))
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9', char == '-', char == '.':
			result.WriteRune(char)
		default:
			result.WriteByte('-')
		}
	}
	cleaned := strings.Trim(result.String(), "-.")
	if cleaned != "" {
		return cleaned
	}
	return fallback
}
