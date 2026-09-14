package webhook

import (
	"github.com/w7panel/w7panel/common/helper"
	corev1 "k8s.io/api/core/v1"
)

const (
	rootCAInjectionAnnotation     = "w7.cc/inject-root-ca"
	rootCABundleImageAnnotation   = "w7.cc/root-ca-bundle-image"
	rootCASourceVolumeName        = "w7panel-root-ca-source"
	rootCABundleVolumeName        = "w7panel-root-ca"
	rootCABundleInitContainerName = "w7panel-root-ca-bundle"
	rootCAIssuerName              = "w7panel-root-ca-issuer"
	rootCAFileName                = "ca.crt"
	rootCASourceMountDirectory    = "/var/run/w7panel-root-ca-source"
	rootCASourceMountPath         = rootCASourceMountDirectory + "/" + rootCAFileName
	rootCAMountDirectory          = "/var/run/w7panel-root-ca"
	rootCAMountPath               = rootCAMountDirectory + "/" + rootCAFileName
	rootCASSLCertDir              = "/etc/ssl/certs:/etc/pki/tls/certs"
)

const rootCABundleCommand = `set -eu
test -s /etc/ssl/certs/ca-certificates.crt
test -s /var/run/w7panel-root-ca-source/ca.crt
cat /etc/ssl/certs/ca-certificates.crt > /var/run/w7panel-root-ca/ca.crt
printf '\n' >> /var/run/w7panel-root-ca/ca.crt
cat /var/run/w7panel-root-ca-source/ca.crt >> /var/run/w7panel-root-ca/ca.crt
chmod 0444 /var/run/w7panel-root-ca/ca.crt`

// rootCAEnvironmentVariables covers the common PEM CA entry points used by
// language runtimes and command-line HTTP clients.
var rootCAEnvironmentVariables = []string{
	"SSL_CERT_FILE",                    // Go, OpenSSL, PHP streams, Ruby and others
	"CURL_CA_BUNDLE",                   // curl CLI
	"REQUESTS_CA_BUNDLE",               // Python requests
	"NODE_EXTRA_CA_CERTS",              // Node.js (adds to the built-in roots)
	"GIT_SSL_CAINFO",                   // Git HTTPS
	"AWS_CA_BUNDLE",                    // AWS CLI and SDKs
	"GRPC_DEFAULT_SSL_ROOTS_FILE_PATH", // gRPC C-core based clients
}

func isRootCAInjectionEnabled(pod *corev1.Pod) bool {
	return pod != nil && pod.Annotations[rootCAInjectionAnnotation] == "true"
}

func injectRootCA(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	modified := false
	if !hasVolume(pod.Spec.Volumes, rootCASourceVolumeName) {
		readOnly := true
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: rootCASourceVolumeName,
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
	if !hasVolume(pod.Spec.Volumes, rootCABundleVolumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: rootCABundleVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
		modified = true
	}

	if !hasContainer(pod.Spec.InitContainers, rootCABundleInitContainerName) {
		bundleInitContainer := newRootCABundleInitContainer(rootCABundleImage(pod))
		// The bundle must exist before regular init containers and native
		// sidecars (init containers with restartPolicy: Always) start.
		pod.Spec.InitContainers = append([]corev1.Container{bundleInitContainer}, pod.Spec.InitContainers...)
		modified = true
	}

	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == rootCABundleInitContainerName {
			continue
		}
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
	if !hasVolumeMount(container.VolumeMounts, rootCABundleVolumeName, rootCAMountPath) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      rootCABundleVolumeName,
			MountPath: rootCAMountPath,
			SubPath:   rootCAFileName,
			ReadOnly:  true,
		})
		modified = true
	}

	for _, name := range rootCAEnvironmentVariables {
		modified = ensureContainerEnvValue(container, name, rootCAMountPath, true) || modified
	}
	// Preserve an explicitly configured OpenSSL certificate directory. The
	// default covers Debian/Alpine and RHEL-family images and keeps their public
	// CA directories available alongside the injected CA file.
	modified = ensureContainerEnvValue(container, "SSL_CERT_DIR", rootCASSLCertDir, false) || modified
	return modified
}

func rootCABundleImage(pod *corev1.Pod) string {
	if pod != nil && pod.Annotations[rootCABundleImageAnnotation] != "" {
		return pod.Annotations[rootCABundleImageAnnotation]
	}
	// The root CA injection annotation is a generic Pod capability. Use the
	// panel image, whose shell and public CA bundle are known, instead of
	// coupling injection to a particular sidecar or arbitrary workload image.
	return helper.SelfImage()
}

func newRootCABundleInitContainer(image string) corev1.Container {
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true
	return corev1.Container{
		Name:            rootCABundleInitContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-ec", rootCABundleCommand},
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      rootCASourceVolumeName,
				MountPath: rootCASourceMountPath,
				SubPath:   rootCAFileName,
				ReadOnly:  true,
			},
			{
				Name:      rootCABundleVolumeName,
				MountPath: rootCAMountDirectory,
			},
		},
	}
}

func hasContainer(containers []corev1.Container, name string) bool {
	for _, container := range containers {
		if container.Name == name {
			return true
		}
	}
	return false
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
