package pid

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestParseFileMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int32
		wantErr bool
	}{
		{name: "octal", input: "0644", want: 0o644},
		{name: "decimal", input: "420", want: 420},
		{name: "empty", input: "", wantErr: true},
		{name: "invalid", input: "08ab", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFileMode(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got mode=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %d, got %d", tt.want, got)
			}
		})
	}
}

func TestApplyModeToMountedFileConfigMapItem(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "app",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "cfg",
				MountPath: "/etc/app",
			}},
		}},
		Volumes: []corev1.Volume{{
			Name: "cfg",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "demo"},
					Items: []corev1.KeyToPath{{
						Key:  "app.yaml",
						Path: "config.yaml",
					}},
				},
			},
		}},
	}

	if !applyModeToMountedFile(podSpec, "/etc/app/config.yaml", 0o600) {
		t.Fatal("expected mode update to succeed")
	}
	got := podSpec.Volumes[0].ConfigMap.Items[0].Mode
	if got == nil || *got != 0o600 {
		t.Fatalf("expected item mode 0600, got %#v", got)
	}
}

func TestApplyModeToMountedFileProjectedDefaultMode(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "app",
			VolumeMounts: []corev1.VolumeMount{{
				Name:      "projected",
				MountPath: "/var/run/config",
			}},
		}},
		Volumes: []corev1.Volume{{
			Name: "projected",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{{
						Secret: &corev1.SecretProjection{
							LocalObjectReference: corev1.LocalObjectReference{Name: "demo"},
						},
					}},
				},
			},
		}},
	}

	if !applyModeToMountedFile(podSpec, "/var/run/config/token", 0o640) {
		t.Fatal("expected projected mode update to succeed")
	}
	got := podSpec.Volumes[0].Projected.DefaultMode
	if got == nil || *got != 0o640 {
		t.Fatalf("expected projected default mode 0640, got %#v", got)
	}
}

func TestFindCreateMountTarget(t *testing.T) {
	result := &MountFilesResult{
		Mounts: []MountFileDescription{
			{
				ContainerName: "app",
				ContainerType: "container",
				MountPath:     "/etc/app",
				Files: []MountFileEntry{
					{
						Path:         "/etc/app/app.yaml",
						RelativePath: "app.yaml",
						SourceType:   "configMap",
						SourceName:   "demo",
						Key:          "app.yaml",
					},
				},
			},
		},
	}

	target, err := findCreateMountTarget(result, "/etc/app/new.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.ContainerName != "app" || target.ContainerType != "container" || target.MountPath != "/etc/app" {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestAttachCreatedFileToPodSpec(t *testing.T) {
	podSpec := &corev1.PodSpec{
		Containers: []corev1.Container{{
			Name: "app",
		}},
		Volumes: []corev1.Volume{{
			Name: "cfg",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "demo"},
				},
			},
		}},
	}

	err := attachCreatedFileToPodSpec(podSpec, createMountTarget{
		ContainerName: "app",
		ContainerType: "container",
	}, "mountfile-new", "cm-new", "new.yaml", "/etc/app/new.yaml", 0o640)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(podSpec.Volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(podSpec.Volumes))
	}
	if podSpec.Volumes[1].Name != "mountfile-new" {
		t.Fatalf("unexpected volume: %#v", podSpec.Volumes[1])
	}
	if len(podSpec.Volumes[1].ConfigMap.Items) != 1 {
		t.Fatalf("expected 1 configmap item, got %d", len(podSpec.Volumes[1].ConfigMap.Items))
	}
	if podSpec.Volumes[1].ConfigMap.Items[0].Key != "new.yaml" || podSpec.Volumes[1].ConfigMap.Items[0].Path != "new.yaml" {
		t.Fatalf("unexpected item: %#v", podSpec.Volumes[1].ConfigMap.Items[0])
	}
	if len(podSpec.Containers[0].VolumeMounts) != 1 {
		t.Fatalf("expected 1 volume mount, got %d", len(podSpec.Containers[0].VolumeMounts))
	}
	if podSpec.Containers[0].VolumeMounts[0].MountPath != "/etc/app/new.yaml" || podSpec.Containers[0].VolumeMounts[0].SubPath != "new.yaml" {
		t.Fatalf("unexpected volume mount: %#v", podSpec.Containers[0].VolumeMounts[0])
	}
}
