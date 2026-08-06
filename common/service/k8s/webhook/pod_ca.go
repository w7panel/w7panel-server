package webhook

import corev1 "k8s.io/api/core/v1"

const (
	rootCAInjectionAnnotation = "w7.cc/inject-root-ca"
	rootCAVolumeName          = "w7panel-root-ca"
	rootCAIssuerName          = "w7panel-root-ca-issuer"
	rootCAMountDirectory      = "/var/run/w7panel-root-ca"
	rootCAFileName            = "ca.crt"
	rootCAMountPath           = rootCAMountDirectory + "/" + rootCAFileName
	rootCASSLCertDir          = "/etc/ssl/certs:/etc/pki/tls/certs"
)

func isRootCAInjectionEnabled(pod *corev1.Pod) bool {
	return pod != nil && pod.Annotations[rootCAInjectionAnnotation] == "true"
}

func injectRootCA(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	modified := false
	if !hasVolume(pod.Spec.Volumes, rootCAVolumeName) {
		readOnly := true
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: rootCAVolumeName,
			VolumeSource: corev1.VolumeSource{
				CSI: &corev1.CSIVolumeSource{
					Driver:   "csi.cert-manager.io",
					ReadOnly: &readOnly,
					VolumeAttributes: map[string]string{
						"csi.cert-manager.io/issuer-name":  rootCAIssuerName,
						"csi.cert-manager.io/issuer-kind":  "ClusterIssuer",
						"csi.cert-manager.io/issuer-group": "cert-manager.io",
						"csi.cert-manager.io/common-name":  "${POD_NAME}.${POD_NAMESPACE}.w7panel-root-ca",
						"csi.cert-manager.io/duration":     "2160h",
						"csi.cert-manager.io/renew-before": "720h",
						"csi.cert-manager.io/ca-file":      rootCAFileName,
					},
				},
			},
		})
		modified = true
	}

	for i := range pod.Spec.InitContainers {
		modified = injectRootCAIntoContainer(&pod.Spec.InitContainers[i]) || modified
	}
	for i := range pod.Spec.Containers {
		modified = injectRootCAIntoContainer(&pod.Spec.Containers[i]) || modified
	}
	return modified
}

func injectRootCAIntoContainer(container *corev1.Container) bool {
	if container == nil {
		return false
	}
	modified := false
	if !hasVolumeMount(container.VolumeMounts, rootCAVolumeName, rootCAMountPath) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      rootCAVolumeName,
			MountPath: rootCAMountPath,
			SubPath:   rootCAFileName,
			ReadOnly:  true,
		})
		modified = true
	}

	modified = ensureContainerEnvValue(container, "SSL_CERT_FILE", rootCAMountPath, true) || modified
	modified = ensureContainerEnvValue(container, "SSL_CERT_DIR", rootCASSLCertDir, false) || modified
	return modified
}

func ensureContainerEnvValue(container *corev1.Container, name, value string, overwrite bool) bool {
	for i := range container.Env {
		if container.Env[i].Name != name {
			continue
		}
		if !overwrite || (container.Env[i].ValueFrom == nil && container.Env[i].Value == value) {
			return false
		}
		container.Env[i].Value = value
		container.Env[i].ValueFrom = nil
		return true
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
	return true
}
