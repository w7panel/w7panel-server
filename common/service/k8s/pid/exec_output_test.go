package pid

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"
	"k8s.io/client-go/rest"
)

func TestGetPidExecStreams(t *testing.T) {
	warning := "time=\"2026-09-15T02:06:01Z\" level=warning msg=\"Config /etc/crictl.yaml does not exist\"\n"
	for _, tc := range []struct {
		name, stdout, stderr     string
		nsenter, failed, wantErr bool
	}{
		{name: "warnings with nsenter", stdout: "8274\n", stderr: warning, nsenter: true},
		{name: "warnings without nsenter", stdout: "'8274'\r\n", stderr: warning},
		{name: "warning only", stderr: warning, wantErr: true},
		{name: "invalid stdout", stdout: "<no value>", stderr: warning, wantErr: true},
		{name: "failed command with numeric stdout", stdout: "8274", stderr: "container not found", failed: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if r.URL.Path != "/api/v1/namespaces/default/pods/agent/exec" || q.Get("stdin") != "false" || q.Get("tty") != "false" || q.Get("stdout") != "true" || q.Get("stderr") != "true" || q.Get("container") != "agent" {
					t.Errorf("unexpected exec request: %s", r.URL)
				}
				wantCmd := []string{"crictl", "inspect", "--output", "go-template", "--template='{{.info.pid}}'", "container-id"}
				if tc.nsenter {
					wantCmd = append([]string{"nsenter", "-t", "1", "--mount", "--pid", "--"}, wantCmd...)
				}
				if !reflect.DeepEqual(q["command"], wantCmd) {
					t.Errorf("command = %q, want %q", q["command"], wantCmd)
				}
				if _, err := httpstream.Handshake(r, w, []string{"v4.channel.k8s.io"}); err != nil {
					t.Error(err)
					return
				}
				type streamReply struct {
					stream httpstream.Stream
					reply  <-chan struct{}
				}
				incoming := make(chan streamReply, 3)
				conn := spdy.NewResponseUpgrader().UpgradeResponse(w, r, func(s httpstream.Stream, reply <-chan struct{}) error {
					incoming <- streamReply{s, reply}
					return nil
				})
				if conn == nil {
					return
				}
				defer conn.Close()
				streams := map[string]httpstream.Stream{}
				for len(streams) < 3 {
					select {
					case s := <-incoming:
						select {
						case <-s.reply:
						case <-ctx.Done():
							return
						}
						streams[s.stream.Headers().Get(corev1.StreamType)] = s.stream
					case <-ctx.Done():
						return
					}
				}
				for _, kind := range []string{corev1.StreamTypeStdout, corev1.StreamTypeStderr, corev1.StreamTypeError} {
					if streams[kind] == nil {
						t.Errorf("missing stream %s", kind)
						return
					}
				}
				io.WriteString(streams[corev1.StreamTypeStderr], tc.stderr)
				streams[corev1.StreamTypeStderr].Close()
				io.WriteString(streams[corev1.StreamTypeStdout], tc.stdout)
				streams[corev1.StreamTypeStdout].Close()
				status := metav1.Status{Status: metav1.StatusSuccess}
				if tc.failed {
					status = metav1.Status{Status: metav1.StatusFailure, Reason: "NonZeroExitCode", Details: &metav1.StatusDetails{Causes: []metav1.StatusCause{{Type: "ExitCode", Message: "1"}}}}
				}
				json.NewEncoder(streams[corev1.StreamTypeError]).Encode(status)
				streams[corev1.StreamTypeError].Close()
				select {
				case <-conn.CloseChan():
				case <-ctx.Done():
				}
			}))
			defer server.Close()
			sdk, err := k8s.NewForRestConfig(&rest.Config{Host: server.URL}, "default")
			if err != nil {
				t.Fatal(err)
			}
			sdk.Ctx = ctx
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "default"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "agent"}}}}
			got, err := GetPid(pod, "containerd://container-id", tc.nsenter, sdk)
			if tc.wantErr {
				if err == nil || got != 0 {
					t.Fatalf("got PID %d, error %v", got, err)
				}
				if !strings.Contains(err.Error(), strings.TrimSpace(tc.stderr)) {
					t.Fatalf("missing stderr diagnostic: %v", err)
				}
				if tc.failed && !strings.Contains(err.Error(), "exit code 1") {
					t.Fatalf("missing command failure: %v", err)
				}
			} else if err != nil || got != 8274 {
				t.Fatalf("got PID %d, error %v", got, err)
			}
		})
	}
}

func TestBytesToPid(t *testing.T) {
	for _, input := range []string{"8274", "8274\n", "'8274'\n", " \t8274\r\n"} {
		got, err := bytesToPid([]byte(input))
		if err != nil || got != 8274 {
			t.Errorf("%q: PID %d, error %v", input, got, err)
		}
	}
	for _, input := range []string{"", "\n", "0", "-1", "<no value>", "warning 8274", "82\n74", "8274\n9000", "99999999999999999999999999999999"} {
		if _, err := bytesToPid([]byte(input)); err == nil {
			t.Errorf("accepted invalid PID %q", input)
		}
	}
}
