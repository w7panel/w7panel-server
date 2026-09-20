// Package artifacturl validates and fetches external artifact URLs.
package artifacturl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const allowedHostsEnv = "ARTIFACT_ALLOWED_HOSTS"

var defaultAllowedHosts = []string{
	"zpk.w7.cc",
	"zpk.idc.w7.com",
	"cdn.w7.cc",
}

// lookupIP is replaceable by tests.  Keeping DNS resolution in the validation
// path is important: a hostname allow-list alone is not safe when its DNS
// record is changed to a cluster or link-local address.
var lookupIP = net.DefaultResolver.LookupIPAddr

// Validate only accepts configured HTTPS artifact origins whose resolved
// addresses are public. This prevents artifact fetches becoming an SSRF path.
func Validate(ctx context.Context, raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid artifact URL: %w", err)
	}
	if u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return nil, errors.New("artifact URL must be an HTTPS URL without user credentials")
	}
	if u.Port() != "" && u.Port() != "443" {
		return nil, errors.New("artifact URL must use HTTPS port 443")
	}
	if !allowedHost(u.Hostname()) {
		return nil, fmt.Errorf("artifact host %q is not allowed", u.Hostname())
	}
	addresses, err := lookupIP(ctx, u.Hostname())
	if err != nil || len(addresses) == 0 {
		return nil, fmt.Errorf("resolve artifact host: %w", err)
	}
	for _, address := range addresses {
		if !isPublic(address.IP) {
			return nil, fmt.Errorf("artifact host %q resolves to a non-public address", u.Hostname())
		}
	}
	return u, nil
}

func allowedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, candidate := range append(defaultAllowedHosts, strings.Split(os.Getenv(allowedHostsEnv), ",")...) {
		if host == strings.ToLower(strings.TrimSuffix(strings.TrimSpace(candidate), ".")) {
			return true
		}
	}
	return false
}

func isPublic(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}

// Get fetches a validated artifact URL. Redirect targets are independently
// validated and response bodies are capped so callers cannot exhaust memory.
func Get(ctx context.Context, raw string, maxBytes int64) ([]byte, error) {
	if _, err := Validate(ctx, raw); err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			_, err := Validate(req.Context(), req.URL.String())
			return err
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("artifact server returned HTTP %d", response.StatusCode)
	}
	if maxBytes <= 0 {
		maxBytes = 32 << 20
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("artifact response exceeds size limit")
	}
	return body, nil
}
