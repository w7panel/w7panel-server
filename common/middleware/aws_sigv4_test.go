package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	"github.com/gin-gonic/gin"
	apiclientv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/apiclient/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifySigV4Request(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.com/api/v1/pods?label=prod&limit=10", bytes.NewReader([]byte(`{"name":"demo"}`)))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")

	requestTime := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	signAWSRequest(t, req, "client-1", "secret-1", "us-east-1", "execute-api", requestTime)

	if err := verifySigV4Request(req, "secret-1", 5*time.Minute, requestTime); err != nil {
		t.Fatalf("verifySigV4Request returned error: %v", err)
	}
}

func TestVerifySigV4RequestRejectsTamperedRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/pods?limit=10", nil)
	req.Host = "example.com"

	requestTime := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	signAWSRequest(t, req, "client-1", "secret-1", "us-east-1", "execute-api", requestTime)
	req.URL.RawQuery = "limit=20"

	if err := verifySigV4Request(req, "secret-1", 5*time.Minute, requestTime); err == nil {
		t.Fatal("expected tampered request to fail verification")
	}
}

func TestVerifySigV4RequestRejectsExpiredTimestamp(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/pods", nil)
	req.Host = "example.com"

	requestTime := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)
	signAWSRequest(t, req, "client-1", "secret-1", "us-east-1", "execute-api", requestTime)

	err := verifySigV4Request(req, "secret-1", 5*time.Minute, requestTime.Add(10*time.Minute))
	if err == nil {
		t.Fatal("expected clock skew validation to fail")
	}
}

func TestAwsSigV4Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api/v1/clusters", nil)
	req.Host = "example.com"
	requestTime := time.Now().UTC()
	signAWSRequest(t, req, "client-1", "secret-1", "us-east-1", "execute-api", requestTime)

	recorder := httptest.NewRecorder()
	ctx, engine := gin.CreateTestContext(recorder)
	ctx.Request = req

	middleware := AwsSigV4{
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
		c.Status(http.StatusNoContent)
	})

	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status code: %d", recorder.Code)
	}
}

func signAWSRequest(t *testing.T, req *http.Request, accessKeyID, secretKey, region, service string, requestTime time.Time) {
	t.Helper()

	signer := v4.NewSigner(credentials.NewStaticCredentials(accessKeyID, secretKey, ""))
	var payload io.ReadSeeker
	if req.Body == nil {
		payload = bytes.NewReader(nil)
	} else {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		payload = bytes.NewReader(body)
	}

	_, err := signer.Sign(req, payload, service, region, requestTime)
	if err != nil {
		t.Fatalf("sign request: %v", err)
	}
}
