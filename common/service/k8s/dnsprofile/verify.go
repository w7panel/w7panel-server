package dnsprofile

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/alidns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/dnspod"
	"github.com/go-acme/lego/v4/providers/dns/route53"
)

// Credentials uses the same environment variable names as the deployed lego DNS webhook.
func ProviderCredentials(provider string, values map[string]string) (map[string]string, error) {
	fields := map[string][]string{
		"alidns":     {"ALICLOUD_ACCESS_KEY", "ALICLOUD_SECRET_KEY"},
		"dnspod":     {"DNSPOD_API_KEY"},
		"cloudflare": {"CLOUDFLARE_DNS_API_TOKEN"},
		"route53":    {"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_REGION"},
	}[provider]
	if len(fields) == 0 {
		return nil, fmt.Errorf("不支持的 DNS 平台: %s", provider)
	}
	result := make(map[string]string, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(values[field])
		if value == "" {
			return nil, fmt.Errorf("缺少 %s", field)
		}
		result[field] = value
	}
	return result, nil
}

func dnsProvider(provider string, values map[string]string) (challenge.Provider, error) {
	switch provider {
	case "alidns":
		config := alidns.NewDefaultConfig()
		config.APIKey, config.SecretKey = values["ALICLOUD_ACCESS_KEY"], values["ALICLOUD_SECRET_KEY"]
		return alidns.NewDNSProviderConfig(config)
	case "dnspod":
		config := dnspod.NewDefaultConfig()
		config.LoginToken = values["DNSPOD_API_KEY"]
		return dnspod.NewDNSProviderConfig(config)
	case "cloudflare":
		config := cloudflare.NewDefaultConfig()
		config.AuthToken = values["CLOUDFLARE_DNS_API_TOKEN"]
		config.ZoneToken = config.AuthToken
		return cloudflare.NewDNSProviderConfig(config)
	case "route53":
		config := route53.NewDefaultConfig()
		config.AccessKeyID, config.SecretAccessKey = values["AWS_ACCESS_KEY_ID"], values["AWS_SECRET_ACCESS_KEY"]
		config.Region = values["AWS_REGION"]
		return route53.NewDNSProviderConfig(config)
	default:
		return nil, errors.New("不支持的 DNS 平台")
	}
}

// Verify proves that the credential can create a real ACME TXT record and removes only its own value.
func Verify(provider, zone string, values map[string]string) (verifyErr error) {
	zone = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(zone)), ".")
	if !validZone(zone) {
		return errors.New("DNS 区域无效")
	}
	credentials, err := ProviderCredentials(provider, values)
	if err != nil {
		return err
	}
	writer, err := dnsProvider(provider, credentials)
	if err != nil {
		return err
	}
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	keyAuth := hex.EncodeToString(nonce)
	token := hex.EncodeToString(nonce[:8])
	if err := writer.Present(zone, token, keyAuth); err != nil {
		return fmt.Errorf("DNS TXT 写入失败: %w", err)
	}
	// Cleanup targets the unique value generated above; never delete other ACME challenges.
	defer func() { verifyErr = errors.Join(verifyErr, writer.CleanUp(zone, token, keyAuth)) }()
	// shortcut: DNS platform write success does not prove public propagation; add lookup if issuance fails from slow DNS.
	return nil
}

func validZone(zone string) bool {
	if len(zone) > 253 || !strings.Contains(zone, ".") || strings.Contains(zone, "*") {
		return false
	}
	for _, label := range strings.Split(zone, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c != '-' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}
