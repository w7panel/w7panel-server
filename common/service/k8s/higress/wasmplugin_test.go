package higress

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s/higress/client/pkg/apis/extensions/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsWhiteDomainPlugin(t *testing.T) {
	tests := []struct {
		name   string
		plugin *v1alpha1.WasmPlugin
		want   bool
	}{
		{
			name:   "new plugin",
			plugin: &v1alpha1.WasmPlugin{ObjectMeta: metav1.ObjectMeta{Name: whiteDomainPluginName, Namespace: higressSystemNamespace}},
			want:   true,
		},
		{
			name:   "legacy plugin",
			plugin: &v1alpha1.WasmPlugin{ObjectMeta: metav1.ObjectMeta{Name: legacyWhiteDomainPluginName, Namespace: higressSystemNamespace}},
			want:   true,
		},
		{
			name:   "wrong namespace",
			plugin: &v1alpha1.WasmPlugin{ObjectMeta: metav1.ObjectMeta{Name: whiteDomainPluginName, Namespace: "default"}},
			want:   false,
		},
		{name: "nil plugin", plugin: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWhiteDomainPlugin(tt.plugin); got != tt.want {
				t.Fatalf("isWhiteDomainPlugin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckDomain(t *testing.T) {
	CheckHost("abc.w7x.com")
}
