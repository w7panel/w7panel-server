package buildimage

import (
	"testing"

	buildimagev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/buildimage/v1alpha1"
)

func TestBuildImageSpec_GetDockerFilePath(t *testing.T) {
	tests := []struct {
		name string
		spec *BuildImageSpec
		want string
	}{
		{
			name: "default dockerfile path under workspace",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{},
			},
			want: "/workspace/Dockerfile",
		},
		{
			name: "relative dockerfile path under workspace",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerfilePath: "deploy/Dockerfile",
					},
				},
			},
			want: "/workspace/deploy/Dockerfile",
		},
		{
			name: "workspace docker context keeps dockerfile path as is",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerfilePath: "deploy/Dockerfile",
						DockerContext:  "/workspace/project",
					},
				},
			},
			want: "deploy/Dockerfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.GetDockerFilePath(); got != tt.want {
				t.Fatalf("GetDockerFilePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildImageSpec_GetBuildContext(t *testing.T) {
	tests := []struct {
		name string
		spec *BuildImageSpec
		want string
	}{
		{
			name: "empty context uses dockerfile directory",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerfilePath: "deploy/Dockerfile",
					},
				},
			},
			want: "/workspace/deploy",
		},
		{
			name: "dot context maps to workspace root",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerContext: ".",
					},
				},
			},
			want: "/workspace/",
		},
		{
			name: "relative context is joined under workspace",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerContext: "project",
					},
				},
			},
			want: "/workspace/project",
		},
		{
			name: "workspace context is returned directly",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{
					Source: buildimagev1alpha1.Source{
						DockerContext: "/workspace/project",
					},
				},
			},
			want: "/workspace/project",
		},
		{
			name: "empty context with default dockerfile returns workspace root",
			spec: &BuildImageSpec{
				BuildImageSpec: &buildimagev1alpha1.BuildImageSpec{},
			},
			want: "/workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.GetBuildContext(); got != tt.want {
				t.Fatalf("GetBuildContext() = %q, want %q", got, tt.want)
			}
		})
	}
}
