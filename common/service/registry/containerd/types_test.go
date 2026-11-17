package containerd

import "testing"

func TestContainerAddrUsesK3sSocketForAgent(t *testing.T) {
	t.Setenv("LOCAL_MOCK", "false")
	t.Setenv("DEBUG", "false")
	t.Setenv("IS_AGENT", "true")
	t.Setenv("IS_CHILD", "true")

	if got := ContainerAddr(); got != k3sContainerAddr {
		t.Fatalf("ContainerAddr() = %q, want %q", got, k3sContainerAddr)
	}
}
