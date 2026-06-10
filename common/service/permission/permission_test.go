package permission

import (
	"context"
	"encoding/json"
	"testing"

	k3ktypes "github.com/w7panel/w7panel/common/service/k8s/k3k/types"
	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatchAPI(t *testing.T) {
	tests := []struct {
		name   string
		rules  map[string][]string
		method string
		path   string
		want   bool
	}{
		{
			name:   "wildcard allows any verb",
			rules:  map[string][]string{"*": {"*"}},
			method: "PATCH",
			path:   "/panel-api/v1/helm/releases/demo",
			want:   true,
		},
		{
			name:   "prefix wildcard allows child path",
			rules:  map[string][]string{"/panel-api/v1/helm/*": {"get"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   true,
		},
		{
			name:   "empty verbs deny explicit path",
			rules:  map[string][]string{"*": {"*"}, "/panel-api/v1/helm/releases": {}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
		{
			name:   "longest match wins",
			rules:  map[string][]string{"/panel-api/v1/helm/*": {"get"}, "/panel-api/v1/helm/releases": {"delete"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
		{
			name:   "get accepts list",
			rules:  map[string][]string{"/panel-api/v1/helm/releases": {"list"}},
			method: "GET",
			path:   "/panel-api/v1/helm/releases",
			want:   true,
		},
		{
			name:   "post requires create",
			rules:  map[string][]string{"/panel-api/v1/helm/releases": {"update"}},
			method: "POST",
			path:   "/panel-api/v1/helm/releases",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchAPI(tt.rules, tt.method, tt.path); got != tt.want {
				t.Fatalf("MatchAPI() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveForServiceAccountRequiresPermissionName(t *testing.T) {
	_, err := ResolveForServiceAccount(context.Background(), nil, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "no-panel-permission"},
	})
	if err == nil {
		t.Fatal("expected error when serviceaccount has no permission annotation")
	}
}

func TestResolveForServiceAccountFallsBackToAnnotations(t *testing.T) {
	api, _ := json.Marshal(map[string][]string{"/panel-api/v1/apps/*": {"get"}})
	menu, _ := json.Marshal([]string{"app/*"})
	p, err := ResolveForServiceAccount(context.Background(), nil, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "custom-panel-permission",
			Annotations: map[string]string{
				k3ktypes.W7_MENU: string(menu),
				"w7.cc/api":      string(api),
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolveForServiceAccount() error = %v", err)
	}
	if !MatchAPI(p.Spec.API, "GET", "/panel-api/v1/apps/demo") {
		t.Fatal("expected annotation api rules to authorize request")
	}
}

func TestIsBuiltin(t *testing.T) {
	tests := []struct {
		name string
		p    *configv1alpha1.Permission
		want bool
	}{
		{
			name: "spec type builtin",
			p:    &configv1alpha1.Permission{Spec: configv1alpha1.PermissionSpec{Type: "builtin"}},
			want: true,
		},
		{
			name: "legacy typemode in",
			p: &configv1alpha1.Permission{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"typemode": "in"}},
			},
			want: true,
		},
		{
			name: "custom",
			p: &configv1alpha1.Permission{
				Spec:       configv1alpha1.PermissionSpec{Type: "custom"},
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"typemode": "custom"}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBuiltin(tt.p); got != tt.want {
				t.Fatalf("IsBuiltin() = %v, want %v", got, tt.want)
			}
		})
	}
}
