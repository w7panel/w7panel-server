package logic

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

func disabledDependEnvLoadHelmEnv(t *testing.T) {
	client := k8s.NewK8sClientInner()
	d := NewDependEnv(client)
	identifie := "gpu_hami"
	namespace := "default"
	result, err := d.LoadHelmEnv(identifie, namespace)
	if err != nil {
		t.Errorf("LoadHelmEnv failed: %v", err)
	}
	if result.Installed {
		t.Errorf("LoadHelmEnv result.Installed should be false")
	}
	if len(result.Envs) != 0 {
		t.Errorf("LoadHelmEnv result.Envs should be empty")
	}
}

func disabledDependEnvLoadLastVersionEnvMaybeNames(t *testing.T) {
	client := k8s.NewK8sClientInner()
	d := NewDependEnv(client)
	name := "w7-pros-28694-jyvtanqm9x"
	namespace := "default"
	result, err := d.LoadLastVersionEnv(name, namespace)
	if err != nil {
		t.Errorf("LoadLastVersionEnv failed: %v", err)
	}
	if !result.Installed {
		t.Errorf("LoadLastVersionEnv result.Installed should be true for maybe names case")
	}
}

func disabledDependEnvLoadLastVersionEnvNotFound(t *testing.T) {
	client := k8s.NewK8sClientInner()
	d := NewDependEnv(client)
	name := "non-existent"
	namespace := "default"
	_, err := d.LoadLastVersionEnv(name, namespace)
	if err == nil {
		t.Errorf("LoadLastVersionEnv should return error for non-existent resource")
	}
}

func disabledDependEnvLoadLastVersionEnvDeployment(t *testing.T) {
	client := k8s.NewK8sClientInner()
	d := NewDependEnv(client)
	name := "test-deployment"
	namespace := "default"
	result, err := d.LoadLastVersionEnv(name, namespace)
	if err != nil {
		t.Errorf("LoadLastVersionEnv failed: %v", err)
	}
	if !result.Installed {
		t.Errorf("LoadLastVersionEnv result.Installed should be true for deployment")
	}
}

func disabledDependEnvLoadLastVersionEnvHelm(t *testing.T) {
	client := k8s.NewK8sClientInner()
	d := NewDependEnv(client)
	name := "test-helm"
	namespace := "default"
	result, err := d.LoadLastVersionEnv(name, namespace)
	if err != nil {
		t.Errorf("LoadLastVersionEnv failed: %v", err)
	}
	if result.Installed {
		t.Errorf("LoadLastVersionEnv result.Installed should be false for helm")
	}
}
