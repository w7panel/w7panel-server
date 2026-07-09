package appgroup

import (
	"context"
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// cert-manager cert
type Certificate struct {
	metav1.TypeMeta
	metav1.ObjectMeta
}

func (c *Certificate) DeepCopyObject() runtime.Object {
	return &Certificate{
		TypeMeta:   c.TypeMeta,
		ObjectMeta: c.ObjectMeta,
	}
}

var schemeGroupVersion = schema.GroupVersion{Group: "cert-manager.io", Version: "v1"}

func init() {
	k8s.GetScheme().AddKnownTypes(schemeGroupVersion, &Certificate{})
	// metav1.Reg
}

func delCert(client sigclient.Client, namespace, certName string) {
	cert := &Certificate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Certificate",
			APIVersion: "cert-manager.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      certName,
			Namespace: namespace,
		},
	}
	err := client.Delete(context.Background(), cert)
	if err != nil {
		slog.Error("delete cert error", "err", err)
	}
}

// 旧绪有Autossl ingress被添加了 new不autossl 就同步删除certmanager
func checkAutossl(client sigclient.Client, old *networkingv1.Ingress, new *networkingv1.Ingress) {
	if old.Annotations == nil || old.Spec.TLS == nil {
		return
	}
	if old.Spec.TLS != nil && len(old.Spec.TLS) == 0 {
		return
	}
	if old.Annotations["cert-manager.io/cluster-issuer"] == "w7-letsencrypt-prod" {
		if new.Annotations == nil {
			new.Annotations = make(map[string]string)
		}
		_, ok := new.Annotations["cert-manager.io/cluster-issuer"]
		if !ok {
			go delCert(client, old.Namespace, old.Spec.TLS[0].SecretName)
		}
	}
}

// 原先有Autossl ingress被删除了 就同步删除certmanager
func checkAutosslDel(client sigclient.Client, old *networkingv1.Ingress) {
	if old.Annotations == nil || old.Spec.TLS == nil {
		return
	}
	if old.Spec.TLS != nil && len(old.Spec.TLS) == 0 {
		return
	}
	_, ok := old.Annotations["cert-manager.io/cluster-issuer"]
	if ok {
		go delCert(client, old.Namespace, old.Spec.TLS[0].SecretName)
	}
}

func OnUpdateIngress(client sigclient.Client, old *networkingv1.Ingress, new *networkingv1.Ingress) {
	checkAutossl(client, old, new)
}

func OnDeleteIngress(client sigclient.Client, ingress *networkingv1.Ingress) {
	checkAutosslDel(client, ingress) //检查
}
