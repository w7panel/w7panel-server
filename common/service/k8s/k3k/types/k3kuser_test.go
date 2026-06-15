// nolint
package types

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestGetDomainWhiteListDefault(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name:        "missing",
			annotations: map[string]string{},
			want:        "[]",
		},
		{
			name:        "empty",
			annotations: map[string]string{W7_DOMAIN_WHITE_LIST: ""},
			want:        "[]",
		},
		{
			name:        "null string",
			annotations: map[string]string{W7_DOMAIN_WHITE_LIST: "null"},
			want:        "[]",
		},
		{
			name:        "configured",
			annotations: map[string]string{W7_DOMAIN_WHITE_LIST: `["w7.cc"]`},
			want:        `["w7.cc"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := NewK3kUser(&v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
				Annotations: tt.annotations,
				Labels:      map[string]string{},
			}})

			if got := user.GetDomainWhiteList(); got != tt.want {
				t.Fatalf("GetDomainWhiteList() = %q, want %q", got, tt.want)
			}
			if got := user.ToArray()[W7_DOMAIN_WHITE_LIST]; got != tt.want {
				t.Fatalf("ToArray()[%q] = %q, want %q", W7_DOMAIN_WHITE_LIST, got, tt.want)
			}
		})
	}
}

func TestGetClusterStorageRequestSize(t *testing.T) {
	sdk := k8s.NewK8sClient().Sdk
	client, err := sdk.ToSigClient()
	if err != nil {
		t.Error(err)
		return
	}
	sa := &v1.ServiceAccount{}
	err = client.Get(sdk.Ctx, types.NamespacedName{Namespace: "default", Name: "g1"}, sa)
	if err != nil {
		t.Error(err)
		return
	}
	k3kUser := NewK3kUser(sa)
	price, err := k3kUser.GetBasePrice()
	if err != nil {
		t.Error(err)
		return
	}
	pstr := price.String()
	t.Log(pstr)
	buy := k3kUser.NeedCreateOrder()
	t.Log(buy)
	// size := k3kUser.GetClusterSysStorageRequestSize()
	// defaultSize := k3kUser.GetStorageRequestSize()
	t1 := k3kUser.GetLimitRange().GetHardRequestStorage().String()
	t2 := k3kUser.GetLimitRange().GetHardRequestStorage().ScaledValue(resource.Kilo)
	t.Log(t1, t2)
	scName := k3kUser.GetStorageClass()
	t.Log(scName)
}

func TestRenew(t *testing.T) {
	sdk := k8s.NewK8sClient().Sdk
	client, err := sdk.ToSigClient()
	if err != nil {
		t.Error(err)
		return
	}
	sa := &v1.ServiceAccount{}
	err = client.Get(sdk.Ctx, types.NamespacedName{Namespace: "default", Name: "console-164315"}, sa)
	if err != nil {
		t.Error(err)
		return
	}
	k3kUser := NewK3kUser(sa)
	ok := k3kUser.NeedRenew()
	if ok {
		t.Log(ok)
	}
}
