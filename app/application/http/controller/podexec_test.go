package controller

import (
	"os"
	"reflect"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/agentpod"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/cp"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func TestNodeTtyTarget(t *testing.T) {
	target, err := nodeTtyTarget("2001:db8::1")
	if err != nil {
		t.Fatal(err)
	}
	if target.Scheme != "http" || target.Host != "[2001:db8::1]:8000" {
		t.Fatalf("unexpected target: %s", target)
	}
	if _, err := nodeTtyTarget("host.example"); err == nil {
		t.Fatal("expected invalid hostIp to fail")
	}
}

func TestNodeTtyShell(t *testing.T) {
	for _, shell := range []string{"/bin/sh", "/bin/bash"} {
		want := []string{"nsenter", "-t", "1", "--mount", "--uts", "--ipc", "--net", "--pid", "--", shell}
		if got := nodeTtyShell(shell); !reflect.DeepEqual(got, want) {
			t.Fatalf("command = %q, want %q", got, want)
		}
	}
}

func TestRegisteredNodeTtyTarget(t *testing.T) {
	if err := agentpod.Register("2001:db8::2", "2001:db8::3"); err != nil {
		t.Fatal(err)
	}
	target, err := nodeTtyForwardTarget("2001:db8::2")
	if err != nil {
		t.Fatal(err)
	}
	if target.Host != "[2001:db8::3]:8000" {
		t.Fatalf("target host = %q", target.Host)
	}
	if err := agentpod.Register("2001:db8::2", "2001:db8::4"); err != nil {
		t.Fatal(err)
	}
	target, err = nodeTtyForwardTarget("2001:db8::2")
	if err != nil || target.Host != "[2001:db8::4]:8000" {
		t.Fatalf("updated target = %v, %v", target, err)
	}
	if err := agentpod.Register("invalid", "10.0.0.3"); err == nil {
		t.Fatal("expected invalid IP registration to fail")
	}
}

func disabledPodExec_KubectlCp(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := k8s.NewK8sClient().PodExecClient()

			factory := cmdutil.NewFactory(client)
			// inb := []byte("")
			// out := os.Stdout
			// in := os.Stdin
			// stream := genericclioptions.IOStreams{
			// 	In:     in,
			// 	Out:    out,
			// 	ErrOut: out,
			// }
			// reader, writer := io.Pipe()
			stream := genericclioptions.IOStreams{
				In:     os.Stdin,
				Out:    os.Stdout,
				ErrOut: os.Stderr,
			}

			cmd := cp.NewCmdCp(factory, stream)
			// cmd.SetArgs([]string{"k8s-offline-8484474cb4-8mrqq:/tmp/test.txt", "/tmp/test.txt"})
			// // cmd.SetOutput(out)
			cmd.SetArgs([]string{"/tmp/test.txt", "k8s-offline-8484474cb4-8mrqq:/tmp/test.txt"})

			err := cmd.Execute()
			// restConfig, _ := client.ToRESTConfig()

			// copyOptions := cp.NewCopyOptions(stream)
			// copyOptions.Complete(factory, cmd, args)
			// copyOptions.Clientset = client.ClientSet
			// copyOptions.ClientConfig = restConfig
			// copyOptions.Container = "k8s-offline"

			// err := copyOptions.Run()

			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
