package appgroup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/w7panel/w7panel/common/service/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestFixSvcNamesLegacyPortsAndRetriesWriteErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		existing  bool
		failWrite bool
		writeVerb string
	}{
		{name: "update", existing: true, writeVerb: http.MethodPut},
		{name: "create", writeVerb: http.MethodPost},
		{name: "retry update error", existing: true, failWrite: true, writeVerb: http.MethodPut},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writes := 0
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				status := http.StatusOK
				body := `{"apiVersion":"v1","kind":"Service","metadata":{"name":"demo","namespace":"default","resourceVersion":"1"},"spec":{"type":"ClusterIP","ports":[{"port":80,"protocol":"TCP"}]}}`
				switch r.Method {
				case http.MethodDelete:
					status, body = http.StatusNotFound, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`
				case http.MethodGet:
					if !tt.existing {
						status, body = http.StatusNotFound, `{"kind":"Status","status":"Failure","reason":"NotFound","code":404}`
					}
				case tt.writeVerb:
					writes++
					var svc corev1.Service
					require.NoError(t, json.NewDecoder(r.Body).Decode(&svc))
					require.Equal(t, []string{"tcp-80", "port-83"}, []string{svc.Spec.Ports[0].Name, svc.Spec.Ports[1].Name})
					if tt.failWrite {
						status, body = http.StatusInternalServerError, `{"kind":"Status","status":"Failure","reason":"InternalError","code":500}`
					}
				default:
					t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})
			clientset, err := kubernetes.NewForConfig(&rest.Config{Host: "https://cluster.test", Transport: transport, ContentConfig: rest.ContentConfig{ContentType: "application/json"}})
			require.NoError(t, err)
			manager := &WorkloadManager{sdk: &k8s.Sdk{ClientSet: clientset, Ctx: context.Background()}}
			workload := NewWorkloadWrapper(&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default", Annotations: map[string]string{"w7.cc/create-svc": "true"}},
				Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Ports: []corev1.ContainerPort{{ContainerPort: 80, Protocol: corev1.ProtocolTCP}, {Name: "port-83", ContainerPort: 83, Protocol: corev1.ProtocolTCP}},
				}}}}},
			})
			err = manager.fixSvc(workload, false)
			if tt.failWrite {
				require.ErrorContains(t, err, "update service default/demo")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, writes)
		})
	}
}
