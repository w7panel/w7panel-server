package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/w7panel/w7panel/common/service/panelauth"
	microappsetting "github.com/w7panel/w7panel/k8s/pkg/apis/microappsetting/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMainSettingsRequiresEmbeddedCKMSession(t *testing.T) {
	for _, principal := range []any{nil, panelauth.Principal{TokenUse: panelauth.TokenUsePanel}} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/panel-api/v1/auth/main-settings", nil)
		if principal != nil {
			ctx.Set("panel_principal", principal)
		}
		MainSettings{}.Get(ctx)
		if ctx.Writer.Status() != http.StatusForbidden {
			t.Fatalf("principal %v received status %d", principal, ctx.Writer.Status())
		}
	}
}

func TestMainSettingsDisplayOnlyIncludesReferencedFields(t *testing.T) {
	setting := &microappsetting.MicroAppSetting{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default", Annotations: map[string]string{"private-setting": "secret-setting"}},
		Spec: microappsetting.MicroAppSettingSpec{
			Login: microappsetting.LoginSettings{ProtocolConfig: microappsetting.ProtocolConfig{
				UserAgreement: microappsetting.ConfigMapRef{Name: "public", Key: "agreement"},
				PrivacyPolicy: microappsetting.ConfigMapRef{Name: "private", Key: "privacy"},
			}},
			General: microappsetting.GeneralSettings{SiteName: "主站", SiteLogo: microappsetting.ConfigMapRef{Name: "public", Key: "logo"}},
		},
	}
	public := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"w7.cc/noauth": "true"}, Annotations: map[string]string{"w7.cc/logo-imagetype": "data:image/png;base64,", "private-config": "secret-config"}},
		Data:       map[string]string{"agreement": "协议", "unreferenced": "secret-key"},
		BinaryData: map[string][]byte{"logo": {1, 2, 3}},
	}
	private := &corev1.ConfigMap{Data: map[string]string{"privacy": "secret-private"}}
	domain := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"annotations": map[string]interface{}{"private-domain": "secret-domain"}},
		"spec":     map[string]interface{}{"type": "A", "ips": []interface{}{"1.2.3.4"}, "unreferenced": "secret-domain-spec"},
		"status":   map[string]interface{}{"private": "secret-status"},
	}}
	encoded, err := json.Marshal(mainSettingsDisplay(setting, map[string]*corev1.ConfigMap{"public": public, "private": private}, domain))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"private-setting", "private-config", "secret-key", "secret-private", "private-domain", "secret-domain-spec", "secret-status", "metadata", "annotations", "status", "resourceVersion", "managedFields"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("response exposed %q: %s", forbidden, encoded)
		}
	}
	var result struct {
		SiteSetting struct {
			Spec struct {
				General struct {
					SiteName string `json:"siteName"`
				} `json:"general"`
			} `json:"spec"`
		} `json:"siteSetting"`
		ConfigMaps map[string]struct {
			Data          map[string]string `json:"data"`
			BinaryData    map[string][]byte `json:"binaryData"`
			LogoImageType string            `json:"logoImageType"`
		} `json:"configMaps"`
		DomainParse struct {
			Spec struct {
				Type string   `json:"type"`
				IPs  []string `json:"ips"`
			} `json:"spec"`
		} `json:"domainParse"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.SiteSetting.Spec.General.SiteName != "主站" || result.ConfigMaps["public"].Data["agreement"] != "协议" || len(result.ConfigMaps) != 1 || result.ConfigMaps["public"].LogoImageType != "data:image/png;base64," || result.DomainParse.Spec.Type != "A" || len(result.DomainParse.Spec.IPs) != 1 {
		t.Fatalf("required display fields missing: %s", encoded)
	}
	missingDomain, err := json.Marshal(mainSettingsDisplay(setting, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(missingDomain), `"domainParse":null`) {
		t.Fatalf("missing domain must remain null: %s", missingDomain)
	}
}
