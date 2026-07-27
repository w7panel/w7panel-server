package microapp

import (
	"testing"

	zpktypes "github.com/w7panel/w7panel/common/service/k8s/zpk/types"
	microappv1 "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestMicroAppRoleCount tests the RoleCount method used in ListTop
func TestMicroAppRoleCount(t *testing.T) {

}

func TestIsPluginMicroApp(t *testing.T) {
	tests := []struct {
		name string
		item microappv1.MicroApp
		want bool
	}{
		{
			name: "plugin microapp",
			item: microappv1.MicroApp{ObjectMeta: metav1.ObjectMeta{
				Name:        "rate-limit",
				Annotations: map[string]string{zpktypes.HELM_APPLICATION_TYPE: "gateway-plugin"},
			}},
			want: true,
		},
		{
			name: "normal microapp",
			item: microappv1.MicroApp{ObjectMeta: metav1.ObjectMeta{
				Name:        "store",
				Annotations: map[string]string{zpktypes.HELM_APPLICATION_TYPE: "native"},
			}},
			want: false,
		},
		{
			name: "microapp without manifest type",
			item: microappv1.MicroApp{ObjectMeta: metav1.ObjectMeta{
				Name: "legacy-app",
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPluginMicroApp(tt.item); got != tt.want {
				t.Fatalf("isPluginMicroApp() = %v, want %v", got, tt.want)
			}
		})
	}
}
