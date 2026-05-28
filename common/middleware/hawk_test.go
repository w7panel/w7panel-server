package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyHawkRequest(t *testing.T) {
	body := []byte(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/v1/pods?label=prod&limit=10", bytes.NewReader(body))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	requestTime := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	signHawkRequest(t, req, "client-1", "secret-1", requestTime, true)

	if err := verifyHawkRequest(req, "secret-1", 5*time.Minute, requestTime); err != nil {
		t.Fatalf("verifyHawkRequest returned error: %v", err)
	}
}

func TestVerifyHawkRequestRejectsTamperedRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/pods?limit=10", nil)
	req.Host = "example.com"

	requestTime := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	signHawkRequest(t, req, "client-1", "secret-1", requestTime, false)
	req.URL.RawQuery = "limit=20"

	if err := verifyHawkRequest(req, "secret-1", 5*time.Minute, requestTime); err == nil {
		t.Fatal("expected tampered request to fail verification")
	}
}

func TestVerifyHawkRequestRejectsExpiredTimestamp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/pods", nil)
	req.Host = "example.com"

	requestTime := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	signHawkRequest(t, req, "client-1", "secret-1", requestTime, false)

	err := verifyHawkRequest(req, "secret-1", 5*time.Minute, requestTime.Add(10*time.Minute))
	if err == nil {
		t.Fatal("expected clock skew validation to fail")
	}
}

func TestVerifyHawkRequestRejectsBodyHashMismatch(t *testing.T) {
	body := []byte(`{"name":"demo"}`)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/v1/pods", bytes.NewReader(body))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")

	requestTime := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	signHawkRequest(t, req, "client-1", "secret-1", requestTime, true)

	req.Body = ioNopCloserBytes([]byte(`{"name":"changed"}`))

	if err := verifyHawkRequest(req, "secret-1", 5*time.Minute, requestTime); err == nil {
		t.Fatal("expected payload hash mismatch")
	}
}

func TestHawkMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/clusters", nil)
	req.Host = "example.com"
	requestTime := time.Now().UTC()
	signHawkRequest(t, req, "client-1", "secret-1", requestTime, false)

	recorder := httptest.NewRecorder()
	ctx, engine := gin.CreateTestContext(recorder)
	ctx.Request = req

	middleware := Hawk{
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
	}

	engine.Use(middleware.Process)
	engine.GET("/api/v1/clusters", func(c *gin.Context) {
		if c.GetString("api_client_id") != "client-1" {
			t.Fatalf("unexpected api_client_id: %s", c.GetString("api_client_id"))
		}
		if c.GetString("hawk_client_id") != "client-1" {
			t.Fatalf("unexpected hawk_client_id: %s", c.GetString("hawk_client_id"))
		}
		c.Status(http.StatusNoContent)
	})

	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}
}

func signHawkRequest(t *testing.T, req *http.Request, id, secret string, requestTime time.Time, includeHash bool) {
	t.Helper()

	authHeader := &hawkAuthHeader{
		ID:    id,
		TS:    requestTime.UTC().Unix(),
		Nonce: "nonce-123",
		Ext:   "",
	}

	if includeHash {
		hash, err := computeHawkPayloadHash(req)
		if err != nil {
			t.Fatalf("computeHawkPayloadHash returned error: %v", err)
		}
		authHeader.Hash = hash
	}

	normalized, err := buildHawkNormalizedString(req, authHeader)
	if err != nil {
		t.Fatalf("buildHawkNormalizedString returned error: %v", err)
	}
	authHeader.MAC = computeHawkMAC(secret, normalized)

	req.Header.Set("Authorization", buildHawkAuthorizationHeader(map[string]string{
		"id":    authHeader.ID,
		"ts":    strconv.FormatInt(authHeader.TS, 10),
		"nonce": authHeader.Nonce,
		"hash":  authHeader.Hash,
		"mac":   authHeader.MAC,
	}))
}

func ioNopCloserBytes(body []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(body))
}
