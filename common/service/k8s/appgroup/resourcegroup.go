package appgroup

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	visibleGroupsKey = "w7.cc/visible-groups"
)

func getResourceGroupName(labels map[string]string) string {
	if labels == nil {
		return ""
	}
	if labels["group"] != "" {
		return labels["group"]
	}
	if labels["w7.cc/group-name"] != "" {
		return labels["w7.cc/group-name"]
	}
	if labels["w7.cc/release-name"] != "" {
		return labels["w7.cc/release-name"]
	}
	if labels["app.kubernetes.io/instance"] != "" {
		return labels["app.kubernetes.io/instance"]
	}
	return labels["w7.cc/suffix"]
}

func getResourceGroupNames(obj metav1.Object) []string {
	if obj == nil {
		return nil
	}
	var names []string
	add := func(values ...string) {
		for _, value := range values {
			if value != "" && !containsString(names, value) {
				names = append(names, value)
			}
		}
	}
	labels := obj.GetLabels()
	if labels != nil {
		add(labels["group"], labels["w7.cc/group-name"], labels["w7.cc/release-name"], labels["app.kubernetes.io/instance"], labels["w7.cc/suffix"])
		add(splitGroupNames(labels[visibleGroupsKey])...)
	}
	annotations := obj.GetAnnotations()
	if annotations != nil {
		add(annotations["meta.helm.sh/release-name"])
	}
	return names
}

func splitGroupNames(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func resourceVisibleInGroup(obj metav1.Object, groupName string) bool {
	return containsString(getResourceGroupNames(obj), groupName)
}
