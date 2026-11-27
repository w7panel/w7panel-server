package logic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RegistryAuthenticationError asks the installer for source repository credentials.
type RegistryAuthenticationError struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}

func (e *RegistryAuthenticationError) Error() string {
	return "源仓库需要登录，或当前账号没有拉取权限"
}

func (self *repo) SetRegistryAuth(auth string) { self.registryAuth = auth }

func registryBearer(ctx context.Context, source, challenge, auth string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", errors.New("unsupported registry authentication challenge")
	}
	fields := map[string]string{}
	for _, field := range regexp.MustCompile(`([a-z]+)="([^"\r\n]*)"`).FindAllStringSubmatch(challenge, -1) {
		fields[field[1]] = field[2]
	}
	origin, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	realm, err := url.Parse(fields["realm"])
	if err != nil || realm.User != nil || realm.Fragment != "" || !strings.EqualFold(realm.Host, origin.Host) || realm.Scheme != origin.Scheme || (realm.Scheme != "https" && realm.Scheme != "http") {
		return "", errors.New("invalid registry authentication realm")
	}
	scope := fields["scope"]
	if fields["service"] != "docker-registry" || !strings.HasPrefix(scope, "repository:") || !strings.HasSuffix(scope, ":pull") || strings.ContainsAny(scope, "\r\n") {
		return "", errors.New("invalid registry pull scope")
	}
	required := &RegistryAuthenticationError{Registry: origin.Host, Repository: strings.TrimSuffix(strings.TrimPrefix(scope, "repository:"), ":pull")}
	query := realm.Query()
	query.Set("scope", scope)
	query.Set("service", fields["service"])
	realm.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	if auth != "" {
		if !strings.HasPrefix(auth, "Basic ") {
			return "", errors.New("invalid registry credential format")
		}
		req.Header.Set("Authorization", auth)
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", required
	}
	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" {
		result.Token = result.AccessToken
	}
	if result.Token == "" {
		return "", required
	}
	return result.Token, nil
}
