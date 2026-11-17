package types

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

const DefaultDomainGroup = "default"

// DomainBinding contains installation input for one manifest ingress group.
type DomainBinding struct {
	Param        string `json:"param"`
	Host         string `json:"host"`
	AutoSSL      bool   `json:"autoSSL"`
	DNSProfileID string `json:"dnsProfileId,omitempty"`
}

var domainGroupParamPattern = regexp.MustCompile(`^DOMAIN_URL(?:_[0-9]+)?$`)

func NormalizeDomainGroupHost(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", nil
	}
	if !strings.Contains(value, "://") {
		value = "http://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil {
		return "", fmt.Errorf("域名格式不正确")
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || strings.Contains(parsed.Host, ":") || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("请填写不含端口、路径或查询参数的域名")
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	var problems []string
	if strings.HasPrefix(host, "*.") {
		problems = validation.IsWildcardDNS1123Subdomain(host)
	} else {
		problems = validation.IsDNS1123Subdomain(host)
	}
	for _, label := range strings.Split(strings.TrimPrefix(host, "*."), ".") {
		if len(label) > 63 {
			return "", fmt.Errorf("域名格式不正确")
		}
	}
	if len(problems) > 0 {
		return "", fmt.Errorf("域名格式不正确")
	}
	return host, nil
}

// PrepareDomainGroups validates against the trusted manifest, then overwrites
// managed values so generic env/Helm input cannot bypass domain validation.
func PrepareDomainGroups(manifest Manifest, option *InstallOption, legacyHost string, legacySSL bool, legacyDNSProfileID string) error {
	groups := []Ingress{}
	known := map[string]bool{}
	for _, group := range manifest.Platform.Ingress {
		if group.DomainParam == "" || len(group.Routes) == 0 {
			continue
		}
		if !domainGroupParamPattern.MatchString(group.DomainParam) || known[group.DomainParam] {
			return fmt.Errorf("域名分组参数不合法或重复")
		}
		known[group.DomainParam] = true
		groups = append(groups, group)
	}
	if len(groups) == 0 && manifest.RequireDomain() {
		required := manifest.RequireDomainForce()
		groups = append(groups, Ingress{Name: DefaultDomainGroup, DomainParam: DefaultDomainGroup, Required: &required, AutoSSL: manifest.RequireDomainHttps()})
		known[DefaultDomainGroup] = true
	}
	if len(groups) == 0 {
		if len(option.DomainGroups) > 0 {
			return fmt.Errorf("制品未声明域名分组")
		}
		return nil
	}
	supplied := map[string]DomainBinding{}
	for _, binding := range option.DomainGroups {
		if !known[binding.Param] {
			return fmt.Errorf("未知的域名分组")
		}
		if _, exists := supplied[binding.Param]; exists {
			return fmt.Errorf("域名分组不能重复")
		}
		supplied[binding.Param] = binding
	}
	if len(option.DomainGroups) == 0 && len(groups) == 1 {
		supplied[groups[0].DomainParam] = DomainBinding{Param: groups[0].DomainParam, Host: legacyHost, AutoSSL: legacySSL, DNSProfileID: legacyDNSProfileID}
	}
	resolved := make([]DomainBinding, 0, len(groups))
	for index, group := range groups {
		binding := supplied[group.DomainParam]
		binding.Param = group.DomainParam
		host, err := NormalizeDomainGroupHost(binding.Host)
		if err != nil {
			return fmt.Errorf("%s：%w", group.Name, err)
		}
		required := index == 0
		if group.Required != nil {
			required = *group.Required
		}
		if required && option.Replicas != 0 && host == "" {
			return fmt.Errorf("请填写%s的域名", group.Name)
		}
		binding.Host = host
		binding.AutoSSL = binding.AutoSSL || group.AutoSSL
		if option.Replicas != 0 && host != "" && binding.AutoSSL && strings.HasPrefix(host, "*.") && binding.DNSProfileID == "" {
			return fmt.Errorf("泛域名自动 HTTPS 需要已验证的 DNS 配置")
		}
		if binding.Param != DefaultDomainGroup {
			filtered := option.EnvKv[:0]
			for _, item := range option.EnvKv {
				if item.Name != binding.Param {
					filtered = append(filtered, item)
				}
			}
			option.EnvKv = append(filtered, EnvKv{Name: binding.Param, Value: host})
		}
		resolved = append(resolved, binding)
	}
	option.DomainGroups = resolved
	if option.HelmValues == nil {
		option.HelmValues = map[string]string{}
	}
	for key := range option.HelmValues {
		if key == "domainGroups" || strings.HasPrefix(key, "domainGroups.") || strings.HasPrefix(key, "domainGroups[") || known[key] {
			delete(option.HelmValues, key)
		}
	}
	return nil
}
