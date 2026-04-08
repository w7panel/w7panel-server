package buildimage

import (
	"github.com/w7panel/w7panel/common/helper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func toBuildJob(spec *BuildImageSpec) (*batchv1.Job, error) {
	registryHost, err := panelRegistryServerHost()
	if err != nil {
		return nil, err
	}
	envs := spec.ToEnv(registryHost)
	envs = append(envs, corev1.EnvVar{
		Name:  "KO_DATA_PATH",
		Value: "/ko-app",
	})
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.GetBuildJobName(),
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app":               spec.GetBuildJobName(),
				"w7.cc/build-image": "true",
				"task-id":           spec.GetBuildJobName(),
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":               spec.GetBuildJobName(),
						"w7.cc/build-image": "true",
						"task-id":           spec.GetBuildJobName(),
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: helper.ServiceAccountName(),
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "build-image",
							Image:           buildImage,
							Env:             envs,
							WorkingDir:      "/workspace",
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"/kaniko/start.sh"},
						},
					},
				},
			},
		},
	}
	return job, nil
}
