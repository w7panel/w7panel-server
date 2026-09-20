package helper

import (
	"net/url"
	"strings"
)

func RemoveQueryParam(rawURL string, keys ...string) string {
	uri, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := uri.Query()
	for _, key := range keys {
		query.Del(key)
	}
	uri.RawQuery = query.Encode()
	return uri.String()
}

// ParseDomainURL normalizes and parses an HTTP(S) domain URL.
func ParseDomainURL(value, defaultScheme string) (*url.URL, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if !strings.Contains(value, "://") {
		value = defaultScheme + strings.Trim(value, "/")
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return nil, false
	}
	return parsed, true
}
