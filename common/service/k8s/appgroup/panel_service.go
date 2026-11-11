package appgroup

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const panelServiceLabel = "w7.cc/panel-service"

func PanelServiceName(name string, uid string) string {
	suffix := strings.ReplaceAll(uid, "-", "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	base := name
	if len(base) > 47 {
		base = strings.TrimRight(base[:47], "-")
	}
	return base + "-w7-" + suffix
}

func ListAppGroupServices(ctx context.Context, client kubernetes.Interface, namespace, group string) ([]corev1.Service, error) {
	list, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]corev1.Service, 0)
	for _, service := range list.Items {
		if resourceVisibleInGroup(&service, group) {
			result = append(result, service)
		}
	}
	return result, nil
}

// ReconcilePanelService is called only after a user saves ports in the panel UI.
func ReconcilePanelService(ctx context.Context, client kubernetes.Interface, workload metav1.Object, kind string, template corev1.PodTemplateSpec) (*corev1.Service, error) {
	if workload.GetUID() == "" {
		return nil, fmt.Errorf("workload UID is required")
	}
	name := PanelServiceName(workload.GetName(), string(workload.GetUID()))
	services := client.CoreV1().Services(workload.GetNamespace())
	current, err := services.Get(ctx, name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	if apierrors.IsNotFound(err) {
		current = nil
	}
	if err == nil && (current.Labels[panelServiceLabel] != "true" || len(current.OwnerReferences) != 1 || current.OwnerReferences[0].UID != workload.GetUID()) {
		return nil, fmt.Errorf("service %s is not owned by this workload", name)
	}
	ports := make([]corev1.ServicePort, 0)
	seen := map[string]bool{}
	for _, container := range template.Spec.Containers {
		for _, port := range container.Ports {
			if port.ContainerPort < 1 || port.ContainerPort > 65535 {
				return nil, fmt.Errorf("invalid container port %d", port.ContainerPort)
			}
			protocol := port.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}
			key := fmt.Sprintf("%s-%d", protocol, port.ContainerPort)
			if seen[key] {
				continue
			}
			seen[key] = true
			ports = append(ports, corev1.ServicePort{Name: strings.ToLower(key), Port: port.ContainerPort, Protocol: protocol})
		}
	}
	if len(ports) == 0 {
		if current == nil {
			return nil, nil
		}
		return nil, services.Delete(ctx, name, metav1.DeleteOptions{})
	}
	if len(template.Labels) == 0 {
		return nil, fmt.Errorf("workload pod labels are required")
	}
	if current == nil {
		labels := map[string]string{panelServiceLabel: "true"}
		for _, key := range []string{"w7.cc/group-name", "w7.cc/release-name", "w7.cc/suffix", "app.kubernetes.io/instance"} {
			if value := workload.GetLabels()[key]; value != "" {
				labels[key] = value
			}
		}
		group := getResourceGroupName(workload.GetLabels())
		if group == "" {
			group = workload.GetAnnotations()["meta.helm.sh/release-name"]
		}
		if group == "" {
			group = workload.GetName()
		}
		labels["group"] = group
		owner := metav1.OwnerReference{APIVersion: "apps/v1", Kind: kind, Name: workload.GetName(), UID: workload.GetUID()}
		return services.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: workload.GetNamespace(), Labels: labels, OwnerReferences: []metav1.OwnerReference{owner}},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: template.Labels, Ports: ports},
		}, metav1.CreateOptions{})
	}
	current.Spec.Selector = template.Labels
	current.Spec.Ports = ports
	return services.Update(ctx, current, metav1.UpdateOptions{})
}
