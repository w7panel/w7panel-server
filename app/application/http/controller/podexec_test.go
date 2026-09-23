package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/w7panel/w7panel/common/service/k8s"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/kubectl/pkg/cmd/cp"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func TestNodeTtyTarget(t *testing.T) {
	target, err := nodeTtyTarget("2001:db8::1", "/bin/bash", "token")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host != "[2001:db8::1]:8000" || parsed.Path != "/panel-api/v1/tty" || parsed.Query().Get("shell") != "/bin/bash" || parsed.Query().Get("api-token") != "token" {
		t.Fatalf("unexpected target: %q", target)
	}
	if _, err := nodeTtyTarget("host.example", "/bin/sh", "token"); err == nil {
		t.Fatal("expected invalid hostIp to fail")
	}
}

func TestRelayNodeTTYForwardsTerminalMessages(t *testing.T) {
	connections := make(chan *websocket.Conn, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := upgrader.Upgrade(writer, request, nil)
		if err == nil {
			connections <- connection
		}
	}))
	defer server.Close()
	endpoint := "ws" + server.URL[len("http"):]
	left, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer left.Close()
	serverLeft := <-connections
	defer serverLeft.Close()
	right, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer right.Close()
	serverRight := <-connections
	defer serverRight.Close()

	done := make(chan struct{}, 2)
	go relayNodeTTY(serverRight, serverLeft, done)
	go relayNodeTTY(serverLeft, serverRight, done)
	if err := left.WriteMessage(websocket.TextMessage, []byte("stdin")); err != nil {
		t.Fatal(err)
	}
	messageType, message, err := right.ReadMessage()
	if err != nil || messageType != websocket.TextMessage || string(message) != "stdin" {
		t.Fatalf("text relay = %d %q %v", messageType, message, err)
	}
	if err := right.WriteMessage(websocket.BinaryMessage, []byte{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	messageType, message, err = left.ReadMessage()
	if err != nil || messageType != websocket.BinaryMessage || string(message) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("binary relay = %d %v %v", messageType, message, err)
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
