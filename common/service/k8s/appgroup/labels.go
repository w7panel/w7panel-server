package appgroup

import (
	"maps"
	"strings"

	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
)

const manifestTypeMetadataKey = "w7.cc/manifest-type"

// SyncDependencyLabels removes stale dependency index labels and rebuilds them
// from the desired AppGroup dependencies.
func SyncDependencyLabels(group *appv1.AppGroup) error {
	desired, err := appv1.DesiredDependencyLabels(group.Spec.Dependencies)
	if err != nil {
		return err
	}
	if group.Labels == nil {
		group.Labels = map[string]string{}
	}
	for key := range group.Labels {
		if strings.HasPrefix(key, appv1.DependencyLabelPrefix) {
			delete(group.Labels, key)
		}
	}
	for key, value := range desired {
		group.Labels[key] = value
	}
	return nil
}

// SyncAppGroupLabels reconciles all labels derived from AppGroup metadata.
// It returns true only when the resulting label set has changed.
func SyncAppGroupLabels(group *appv1.AppGroup) (bool, error) {
	if group == nil {
		return false, nil
	}
	before := maps.Clone(group.Labels)
	if err := SyncDependencyLabels(group); err != nil {
		return false, err
	}
	syncManifestTypeLabel(group)
	return !maps.Equal(before, group.Labels), nil
}

// NeedsSyncAppGroupLabels checks whether label reconciliation is required
// without mutating the informer-owned AppGroup copy.
func NeedsSyncAppGroupLabels(group *appv1.AppGroup) (bool, error) {
	if group == nil {
		return false, nil
	}
	desiredDependencies, err := appv1.DesiredDependencyLabels(group.Spec.Dependencies)
	if err != nil {
		return false, err
	}
	for key, value := range group.Labels {
		if !strings.HasPrefix(key, appv1.DependencyLabelPrefix) {
			continue
		}
		desiredValue, exists := desiredDependencies[key]
		if !exists || desiredValue != value {
			return true, nil
		}
	}
	for key, value := range desiredDependencies {
		if group.Labels[key] != value {
			return true, nil
		}
	}
	manifestType := strings.TrimSpace(group.Annotations[manifestTypeMetadataKey])
	return manifestType != "" && group.Labels[manifestTypeMetadataKey] != manifestType, nil
}

// syncManifestTypeLabel backfills the selector-friendly label for AppGroups
// created before manifest type labels were introduced. A missing or empty
// annotation is intentionally ignored so reconciliation never removes a label
// initialized directly from the manifest during AppGroup creation.
func syncManifestTypeLabel(group *appv1.AppGroup) {
	manifestType := strings.TrimSpace(group.Annotations[manifestTypeMetadataKey])
	if manifestType == "" || group.Labels[manifestTypeMetadataKey] == manifestType {
		return
	}
	if group.Labels == nil {
		group.Labels = map[string]string{}
	}
	group.Labels[manifestTypeMetadataKey] = manifestType
}
