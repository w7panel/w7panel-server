package middleware

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

func TestVerifyConsoleRequestSignatureQuery(t *testing.T) {
	values := url.Values{}
	values.Set("appid", "app-1")
	values.Set("timestamp", "1710000000")
	values.Set("nonce", "nonce-123")
	values.Set("host", "demo.example.com")
	values.Set("sign", signValues(values, "secret-1"))

	req := httptest.NewRequest(http.MethodGet, "/test?"+values.Encode(), nil)
	ok, err := verifyConsoleRequestSignature(req, "app-1", "secret-1")
	if err != nil {
		t.Fatalf("verifyConsoleRequestSignature returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected query signature to be valid")
	}
}

func TestVerifyConsoleRequestSignatureForm(t *testing.T) {
	values := url.Values{}
	values.Set("appid", "app-1")
	values.Set("timestamp", "1710000000")
	values.Set("nonce", "nonce-123")
	values.Set("host", "demo.example.com")
	values.Set("siteIdentifie", "site-1")
	values.Set("sign", signValues(values, "secret-1"))

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ok, err := verifyConsoleRequestSignature(req, "app-1", "secret-1")
	if err != nil {
		t.Fatalf("verifyConsoleRequestSignature returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected form signature to be valid")
	}
}

func TestVerifyConsoleRequestSignatureJSON(t *testing.T) {
	data := map[string]interface{}{
		"appid":       "app-1",
		"timestamp":   "1710000000",
		"nonce":       "nonce-123",
		"host":        "demo.example.com",
		"installId":   "install-1",
		"releaseName": "release-1",
	}
	data["sign"] = signJSON(data, "secret-1")

	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	ok, err := verifyConsoleRequestSignature(req, "app-1", "secret-1")
	if err != nil {
		t.Fatalf("verifyConsoleRequestSignature returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected json signature to be valid")
	}
}

func TestVerifyConsoleRequestSignatureRejectsTamperedRequest(t *testing.T) {
	values := url.Values{}
	values.Set("appid", "app-1")
	values.Set("timestamp", "1710000000")
	values.Set("nonce", "nonce-123")
	values.Set("host", "demo.example.com")
	values.Set("sign", signValues(values, "secret-1"))
	values.Set("host", "changed.example.com")

	req := httptest.NewRequest(http.MethodGet, "/test?"+values.Encode(), nil)
	ok, err := verifyConsoleRequestSignature(req, "app-1", "secret-1")
	if err != nil {
		t.Fatalf("verifyConsoleRequestSignature returned error: %v", err)
	}
	if ok {
		t.Fatal("expected tampered query signature to be invalid")
	}
}

func signValues(values url.Values, secret string) string {
	cloned := url.Values{}
	for key, items := range values {
		for _, item := range items {
			cloned.Add(key, item)
		}
	}
	cloned.Del("sign")

	var keys []string
	for key := range cloned {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	signStr := ""
	for i, key := range keys {
		signStr += fmt.Sprintf("%s=%s", key, url.QueryEscape(cloned.Get(key)))
		if i < len(keys)-1 {
			signStr += "&"
		}
	}

	sum := md5.Sum([]byte(signStr + secret))
	return hex.EncodeToString(sum[:])
}

func signJSON(data map[string]interface{}, secret string) string {
	cloned := map[string]interface{}{}
	for key, value := range data {
		if key == "sign" {
			continue
		}
		cloned[key] = value
	}

	body, err := json.Marshal(cloned)
	if err != nil {
		panic(err)
	}
	sum := md5.Sum([]byte(string(body) + secret))
	return hex.EncodeToString(sum[:])
}
