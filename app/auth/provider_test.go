package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	commonmiddleware "github.com/w7panel/w7panel/common/middleware"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRegisterHawkTestRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	group := engine.Group("/panel-api/v1/auth")
	registerHawkTestRoute(group, commonmiddleware.Hawk{
		MaxSkew: 5 * time.Minute,
		ResolveClient: func(_ context.Context, clientID string) (*apiclientv1alpha1.ApiClient, error) {
			return &apiclientv1alpha1.ApiClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-1"},
				Spec: apiclientv1alpha1.ApiClientSpec{
					Enabled:      true,
					ClientID:     clientID,
					ClientName:   "demo-client",
					ClientSecret: "secret-1",
				},
			}, nil
		},
	}.Process)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/panel-api/v1/auth/hawk-test", nil)
	req.Host = "example.com"
	signHawkRequestForProviderTest(t, req, "client-1", "secret-1", time.Now().UTC())

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}

	var resp struct {
		OK            bool   `json:"ok"`
		APIClientID   string `json:"apiClientId"`
		APIClientName string `json:"apiClientName"`
		HawkClientID  string `json:"hawkClientId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if !resp.OK {
		t.Fatal("expected ok to be true")
	}
	if resp.APIClientID != "client-1" {
		t.Fatalf("unexpected apiClientId: %s", resp.APIClientID)
	}
	if resp.APIClientName != "demo-client" {
		t.Fatalf("unexpected apiClientName: %s", resp.APIClientName)
	}
	if resp.HawkClientID != "client-1" {
		t.Fatalf("unexpected hawkClientId: %s", resp.HawkClientID)
	}
}

func TestRegisterHawkTestRouteRejectsMissingAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	group := engine.Group("/panel-api/v1/auth")
	registerHawkTestRoute(group, commonmiddleware.Hawk{
		MaxSkew: 5 * time.Minute,
		ResolveClient: func(_ context.Context, clientID string) (*apiclientv1alpha1.ApiClient, error) {
			return &apiclientv1alpha1.ApiClient{
				ObjectMeta: metav1.ObjectMeta{Name: "client-1"},
				Spec: apiclientv1alpha1.ApiClientSpec{
					Enabled:      true,
					ClientID:     clientID,
					ClientName:   "demo-client",
					ClientSecret: "secret-1",
				},
			}, nil
		},
	}.Process)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/panel-api/v1/auth/hawk-test", nil)
	req.Host = "example.com"

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}
}

func signHawkRequestForProviderTest(t *testing.T, req *http.Request, id, secret string, requestTime time.Time) {
	t.Helper()

	resource := req.URL.EscapedPath()
	if req.URL.RawQuery != "" {
		resource += "?" + req.URL.RawQuery
	}

	normalized := strings.Join([]string{
		"hawk.1.header",
		strconv.FormatInt(requestTime.UTC().Unix(), 10),
		"nonce-123",
		strings.ToUpper(req.Method),
		resource,
		strings.ToLower(req.Host),
		defaultPort(req.URL),
		"",
		"",
		"",
		"",
	}, "\n") + "\n"

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(normalized))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("Authorization", `Hawk id="`+id+`", mac="`+signature+`", nonce="nonce-123", ts="`+strconv.FormatInt(requestTime.UTC().Unix(), 10)+`"`)
}

func defaultPort(u *url.URL) string {
	if u != nil && strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}
