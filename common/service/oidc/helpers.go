package oidc

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"time"
)

func (s *Server) pruneAuthRequestsLocked() {
	now := time.Now()
	for id, req := range s.authRequests {
		if req.CreationDate.Add(s.config.CodeTTL).Before(now) {
			delete(s.authRequests, id)
		}
	}
}

func (s *Server) pruneTokensLocked() {
	now := time.Now()
	for id, token := range s.accessTokens {
		if now.After(token.Expiration) {
			delete(s.accessTokens, id)
		}
	}
	for tokenValue, token := range s.refreshTokens {
		if now.After(token.Expiration) {
			delete(s.refreshTokens, tokenValue)
		}
	}
}

func splitLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func normalizeScopes(scopes []string) []string {
	allowed := map[string]struct{}{
		"openid":         {},
		"profile":        {},
		"offline_access": {},
	}
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := allowed[scope]; ok && !containsString(result, scope) {
			result = append(result, scope)
		}
	}
	return result
}

func normalizeClientID(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "-")
	if strings.HasPrefix(value, "oidc_") {
		value = strings.ReplaceAll(value, "oidc_", "oidc-")
	}
	return value
}

func normalizeAuthMethod(mode, secret string) string {
	if mode != "" {
		return mode
	}
	if secret == "" {
		return "none"
	}
	return "client_secret_basic"
}

func randomToken(length int) string {
	size := length
	if size < 16 {
		size = 16
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:length]
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, item := range left {
		seen[item]++
	}
	for _, item := range right {
		seen[item]--
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}
