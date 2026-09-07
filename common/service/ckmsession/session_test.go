package ckmsession

import (
	"context"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/w7panel/w7panel/common/service/panelauth"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestCKMBootstrapIdentityAndExpiry(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	p := panelauth.Principal{Username: "alice", CVMName: "a", K3KNamespace: "k3k-alice"}
	for _, tc := range []struct {
		name, subject string
		aud           []string
		expires       int64
		want          time.Duration
	}{
		{"valid", "system:serviceaccount:default:normal", Audiences(p), now.Add(time.Hour).Unix(), 10 * time.Minute},
		{"short", "system:serviceaccount:default:normal", Audiences(p), now.Add(time.Minute).Unix(), time.Minute},
		{"expired", "system:serviceaccount:default:normal", Audiences(p), now.Add(-time.Second).Unix(), 0},
		{"wrong-account", "system:serviceaccount:default:founder", Audiences(p), now.Add(time.Hour).Unix(), 0},
		{"wrong-cluster", "system:serviceaccount:default:normal", Audiences(panelauth.Principal{Username: "alice", CVMName: "b", K3KNamespace: "k3k-alice"}), now.Add(time.Hour).Unix(), 0},
		{"wrong-owner", "system:serviceaccount:default:normal", Audiences(panelauth.Principal{Username: "bob", CVMName: "a", K3KNamespace: "k3k-bob"}), now.Add(time.Hour).Unix(), 0},
		{"no-expiry", "system:serviceaccount:default:normal", Audiences(p), 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := jwt.MapClaims{"aud": tc.aud}
			if tc.expires != 0 {
				claims["exp"] = tc.expires
			}
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test"))
			if err != nil {
				t.Fatal(err)
			}
			got, err := validateBootstrap(raw, tc.subject, "default", p, now)
			if got != tc.want || (err != nil) != (tc.want == 0) {
				t.Fatalf("got %s/%v; want %s", got, err, tc.want)
			}
		})
	}
}

func TestCKMTargetMustExistAndNotBeDeleting(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "ckm.w7.cc/v1alpha2", "kind": "Ckm", "metadata": map[string]interface{}{"name": "a", "namespace": "k3k-alice", "uid": "uid-a"}}}
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	got, err := loadTarget(context.Background(), client, "k3k-alice", "a")
	if err != nil || string(got.GetUID()) != "uid-a" {
		t.Fatalf("%v/%v", got, err)
	}
	if _, err = loadTarget(context.Background(), client, "k3k-bob", "a"); err == nil {
		t.Fatal("accepted another namespace")
	}
	if _, err = loadTarget(context.Background(), client, "default", "a"); err == nil {
		t.Fatal("accepted non-CKM namespace")
	}
	obj.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
	client = fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	if _, err = loadTarget(context.Background(), client, "k3k-alice", "a"); err == nil {
		t.Fatal("accepted deleting CKM")
	}
}
