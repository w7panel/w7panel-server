// nolint
package k3k

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/w7corp/sdk-open-cloud-go/service"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	sitev1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/site/v1alpha1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSiteRegistrationOpenID(t *testing.T) {
	tests := []struct {
		name      string
		w7config  *config.W7Config
		configErr error
		want      string
		wantErr   string
	}{
		{name: "config error", configErr: errors.New("not found"), wantErr: "get W7Config: not found"},
		{name: "missing config", wantErr: "W7Config is empty"},
		{name: "missing user info", w7config: &config.W7Config{}, wantErr: "W7Config user info is empty"},
		{name: "missing openid", w7config: &config.W7Config{UserInfo: &service.ResultUserinfo{}}, wantErr: "W7Config user OpenID is empty"},
		{name: "success", w7config: &config.W7Config{UserInfo: &service.ResultUserinfo{OpenId: "openid"}}, want: "openid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := siteRegistrationOpenID(tt.w7config, tt.configErr)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("siteRegistrationOpenID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("OpenID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetSiteRegistrationFailed(t *testing.T) {
	site := &sitev1alpha1.Site{}
	setSiteRegistrationFailed(site, "OpenIDUnavailable", "W7Config user OpenID is empty")

	if site.Status.Phase != "Failed" {
		t.Fatalf("phase = %q, want Failed", site.Status.Phase)
	}
	if site.Status.Message != "W7Config user OpenID is empty" {
		t.Fatalf("message = %q", site.Status.Message)
	}
	if len(site.Status.Conditions) != 1 || site.Status.Conditions[0].Reason != "OpenIDUnavailable" {
		t.Fatalf("conditions = %#v, want OpenIDUnavailable", site.Status.Conditions)
	}
}

func TestSyncIngress(t *testing.T) {
	// os.Setenv("LOCAL_MOCK", "true")
	ing := &K3kSync{
		VirtualName:      "ing-zqyhtpkg",
		VirtualNamespace: "default",
		K3kName:          "console-75780",
		K3kNamespace:     "k3k-console-75780",
		K3kMode:          "virtual",
	}
	err := SyncIngress(ing)
	if err != nil {

		t.Error(err)
	}
}

func TestSyncIngressHttps(t *testing.T) {
	sdk := k8s.NewK8sClient().Sdk
	ing, err := sdk.ClientSet.NetworkingV1().Ingresses("default").Get(sdk.Ctx, "ing-jkfsrckbxs", metav1.GetOptions{})
	if err != nil {
		t.Error(err)
	}
	err = SyncIngressHttp(ing)
	if err != nil {
		t.Error(err)
	}
}

func TestSyncIngressHttp(t *testing.T) {
	// os.Setenv("LOCAL_MOCK", "true")
	os.Setenv("K3K_NAME", "console-164315")
	os.Setenv("K3K_NAMESPACE", "k3k-console-164315")
	os.Setenv("ROOT_POD_IP", "172.16.1.162")
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ing-cyqqsnoq",
			Namespace: "default",
		},
	}
	err := SyncIngressHttp(ing)
	if err != nil {
		t.Error(err)
	}
}

func TestSyncConfigmap(t *testing.T) {
	os.Setenv("LOCAL_MOCK", "true")
	os.Setenv("K3K_NAME", "v56")
	os.Setenv("K3K_NAMESPACE", "k3k-v56")
	k3ksync := &K3kSync{
		VirtualName:      "registries",
		VirtualNamespace: "default",
		K3kName:          "v56",
		K3kNamespace:     "k3k-v56",
	}
	err := SyncConfigmap(k3ksync)
	if err != nil {
		t.Error(err)
	}
}

func TestSyncChild(t *testing.T) {
	secret, err := k8s.NewK8sClient().Sdk.ClientSet.CoreV1().Secrets("k3k-console-75780").Get(context.TODO(), "who8-fan-b2-sz-w7-com-tls-secret", metav1.GetOptions{})
	if err != nil {
		t.Log(err)
		return
	}
	err = SyncToChildSecret(secret)
	if err != nil {
		t.Error(err)
	}
}

//who8-fan-b2-sz-w7-com-tls-secret
