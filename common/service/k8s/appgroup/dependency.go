package appgroup

import (
	"fmt"
	"strings"

	"github.com/w7panel/w7panel/common/service/k8s"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
)

// DependencyReference identifies an AppGroup dependency before its metadata is resolved.
type DependencyReference struct {
	Namespace string
	Name      string
	Identifie string
}

type getAppGroupFunc func(namespace, name string) (*appv1.AppGroup, error)

// ResolveDependencies validates logical AppGroup references and resolves the
// metadata persisted in AppGroup.spec.dependencies.
func ResolveDependencies(client *k8s.Sdk, defaultNamespace string, references []DependencyReference) ([]appv1.AppGroupDependency, error) {
	if len(references) == 0 {
		return nil, nil
	}
	groupAPI, err := NewAppGroupApi(client)
	if err != nil {
		return nil, fmt.Errorf("initialize appgroup dependency client: %w", err)
	}
	return resolveDependencies(defaultNamespace, references, groupAPI.GetAppGroup)
}

func resolveDependencies(defaultNamespace string, references []DependencyReference, getAppGroup getAppGroupFunc) ([]appv1.AppGroupDependency, error) {
	resolved := make([]appv1.AppGroupDependency, 0, len(references))
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		name := strings.TrimSpace(reference.Name)
		if name == "" {
			return nil, fmt.Errorf("dependency %q has no name", reference.Identifie)
		}
		dependencyNamespace := strings.TrimSpace(reference.Namespace)
		if dependencyNamespace == "" {
			dependencyNamespace = defaultNamespace
		}
		target, err := getAppGroup(dependencyNamespace, name)
		if err != nil {
			return nil, fmt.Errorf("resolve dependency %s/%s: %w", dependencyNamespace, name, err)
		}
		if target == nil {
			return nil, fmt.Errorf("resolve dependency %s/%s: empty AppGroup", dependencyNamespace, name)
		}
		if _, err = appv1.DependencyLabelKey(target.Name); err != nil {
			return nil, err
		}
		dependency := appv1.AppGroupDependency{
			Namespace:       target.Namespace,
			Name:            target.Name,
			Identifie:       target.Spec.Identifie,
			ApplicationType: ApplicationType(target),
		}
		if dependency.Identifie == "" {
			dependency.Identifie = strings.TrimSpace(reference.Identifie)
		}
		key := dependency.Namespace + "\x00" + dependency.Name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		resolved = append(resolved, dependency)
	}
	return resolved, nil
}

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

// ApplicationType reads the existing manifest type annotation on an AppGroup.
func ApplicationType(group *appv1.AppGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimSpace(group.Annotations["w7.cc/manifest-type"])
}
