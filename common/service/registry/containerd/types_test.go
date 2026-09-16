package containerd

import "testing"

func TestContainerAddrUsesK3sSocketForChildAgent(t *testing.T) {
	t.Setenv("LOCAL_MOCK", "false")
	t.Setenv("DEBUG", "false")
	t.Setenv("IS_AGENT", "false")
	t.Setenv("IS_CHILD", "true")

	if got := ContainerAddr(); got != k3sContainerAddr {
		t.Fatalf("ContainerAddr() = %q, want %q", got, k3sContainerAddr)
	}
}
