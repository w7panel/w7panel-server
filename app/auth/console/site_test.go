// nolint
package console

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSite_checkTLSHandshake(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	oldHost, oldFactory := sitero.Host, tlsHandshakeClientFactory
	sitero.Host = strings.TrimPrefix(server.URL, "https://")
	tlsHandshakeClientFactory = func(_ string, _ time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		sitero.Host = oldHost
		tlsHandshakeClientFactory = oldFactory
	})

	if got := (Site{}).checkTLSHandshake(); !got {
		t.Fatal("checkTLSHandshake() = false, want true for a trusted local TLS server")
	}
}

func TestSite_checkTLSHandshake_Failure(t *testing.T) {
	oldHost, oldFactory := sitero.Host, tlsHandshakeClientFactory
	sitero.Host = "invalid.host"
	tlsHandshakeClientFactory = func(_ string, _ time.Duration) *http.Client {
		return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("TLS handshake failed")
		})}
	}
	t.Cleanup(func() {
		sitero.Host = oldHost
		tlsHandshakeClientFactory = oldFactory
	})

	s := Site{}
	if got := s.checkTLSHandshake(); got != false {
		t.Errorf("checkTLSHandshake() = %v, want false", got)
	}
}
