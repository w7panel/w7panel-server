package buildimage

import (
	"github.com/w7panel/w7panel/common/helper"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func toBuildJob(spec BuildImageSpec) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.GetBuildJobName(),
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app":     "w7panel-build-image",
				"task-id": spec.GetBuildJobName(),
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":     "w7panel-build-image",
						"task-id": spec.GetBuildJobName(),
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: helper.ServiceAccountName(),
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "build-image",
							Image: helper.SelfImage(),
							// Env:             envs,
							WorkingDir:      "/workspace",
							ImagePullPolicy: corev1.PullAlways,
							Command:         []string{"sh", "-c", "${KO_DATA_PATH}/shell/build-image.sh"},
						},
					},
				},
			},
		},
	}
	return job
}
