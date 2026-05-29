package coredns

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	CoreDNSNamespace      = "kube-system"
	CoreDNSName           = "coredns"
	CoreDNSCustomName     = "coredns-custom"
	PublicDNSServiceName  = "w7-coredns-public"
	DefaultTTL            = 60
	DefaultMXPriority     = 10
	recordIDCommentPrefix = "# w7-dns-record-id:"
)

var (
	domainPartRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	hostPartRe   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

type Zone struct {
	Domain     string    `json:"domain"`
	RecordNum  int       `json:"recordNum"`
	UpdateTime time.Time `json:"updateTime,omitempty"`
}

type Record struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	TTL        int    `json:"ttl"`
	MXPriority int    `json:"mxPriority,omitempty"`
}

type ServerStatus struct {
	Enabled     bool     `json:"enabled"`
	ServiceName string   `json:"serviceName"`
	ServiceType string   `json:"serviceType,omitempty"`
	ExternalIPs []string `json:"externalIPs"`
}

func NormalizeDomain(domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return "", errors.New("domain is invalid")
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return "", errors.New("domain must include at least two labels")
	}
	for _, part := range parts {
		if !domainPartRe.MatchString(part) {
			return "", fmt.Errorf("domain label %q is invalid", part)
		}
	}
	return domain, nil
}

func NormalizeRecord(domain string, record Record) (Record, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return record, err
	}
	record.Name = strings.TrimSpace(strings.ToLower(record.Name))
	if record.Name == "" {
		record.Name = "@"
	}
	if record.Name != "@" {
		parts := strings.Split(strings.TrimSuffix(record.Name, "."), ".")
		for _, part := range parts {
			if !hostPartRe.MatchString(part) {
				return record, fmt.Errorf("record name %q is invalid", record.Name)
			}
		}
		record.Name = strings.Join(parts, ".")
	}
	record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
	record.Value = strings.TrimSpace(record.Value)
	if record.TTL <= 0 {
		record.TTL = DefaultTTL
	}
	if record.MXPriority <= 0 {
		record.MXPriority = DefaultMXPriority
	}
	if err := validateRecordValue(domain, record); err != nil {
		return record, err
	}
	if record.ID == "" {
		record.ID = MakeRecordID(domain, record)
	}
	return record, nil
}

func MakeRecordID(domain string, record Record) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		domain,
		record.Name,
		record.Type,
		record.Value,
		strconv.Itoa(record.TTL),
		strconv.Itoa(record.MXPriority),
	}, "|")))
	return hex.EncodeToString(sum[:])[:12]
}

func FQDN(domain string, name string) string {
	if name == "" || name == "@" {
		return domain + "."
	}
	return strings.TrimSuffix(name, ".") + "." + domain + "."
}

func RenderZone(domain string, records []Record) (string, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", err
	}
	normalized := make([]Record, 0, len(records))
	for _, record := range records {
		item, err := NormalizeRecord(domain, record)
		if err != nil {
			return "", err
		}
		normalized = append(normalized, item)
	}
	sortRecords(normalized)

	var builder strings.Builder
	builder.WriteString(domain)
	builder.WriteString(" {\n")
	for i := 0; i < len(normalized); {
		record := normalized[i]
		qname := FQDN(domain, record.Name)
		builder.WriteString("  template IN ")
		builder.WriteString(record.Type)
		builder.WriteString(" ")
		builder.WriteString(qname)
		builder.WriteString(" {\n")
		for i < len(normalized) && normalized[i].Name == record.Name && normalized[i].Type == record.Type {
			item := normalized[i]
			builder.WriteString("    ")
			builder.WriteString(recordIDCommentPrefix)
			builder.WriteString(" ")
			builder.WriteString(item.ID)
			builder.WriteString("\n")
			builder.WriteString("    answer \"")
			builder.WriteString(escapeAnswer(renderAnswer(qname, item)))
			builder.WriteString("\"\n")
			i++
		}
		builder.WriteString("  }\n\n")
	}
	builder.WriteString("  template ANY ANY {\n")
	builder.WriteString("    rcode NOERROR\n")
	builder.WriteString("  }\n\n")
	builder.WriteString("  loadbalance\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}

func ParseZone(domain string, data string) ([]Record, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(data, "\n")
	records := make([]Record, 0)
	nextID := ""
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, recordIDCommentPrefix) {
			nextID = strings.TrimSpace(strings.TrimPrefix(line, recordIDCommentPrefix))
			continue
		}
		if !strings.HasPrefix(line, "answer ") {
			continue
		}
		answer := strings.TrimSpace(strings.TrimPrefix(line, "answer "))
		answer = strings.Trim(answer, `"`)
		answer = strings.ReplaceAll(answer, `\"`, `"`)
		record, err := parseAnswer(domain, answer)
		if err != nil {
			continue
		}
		if nextID != "" {
			record.ID = nextID
			nextID = ""
		}
		record, err = NormalizeRecord(domain, record)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	sortRecords(records)
	return records, nil
}

func ConfigMapKey(domain string) string {
	return domain + ".server"
}

func DomainFromConfigMapKey(key string) (string, bool) {
	if !strings.HasSuffix(key, ".server") {
		return "", false
	}
	domain := strings.TrimSuffix(key, ".server")
	domain, err := NormalizeDomain(domain)
	return domain, err == nil
}

func sortRecords(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		a := records[i]
		b := records[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		return a.Value < b.Value
	})
}

func validateRecordValue(domain string, record Record) error {
	if record.Value == "" {
		return errors.New("record value is required")
	}
	switch record.Type {
	case "A":
		addr, err := netip.ParseAddr(record.Value)
		if err != nil || !addr.Is4() {
			return errors.New("A record value must be an IPv4 address")
		}
	case "AAAA":
		addr, err := netip.ParseAddr(record.Value)
		if err != nil || !addr.Is6() {
			return errors.New("AAAA record value must be an IPv6 address")
		}
	case "CNAME":
		if _, err := NormalizeDomain(record.Value); err != nil {
			return errors.New("CNAME value must be a domain")
		}
	case "TXT":
		if strings.ContainsAny(record.Value, "\r\n") {
			return errors.New("TXT value must be a single line")
		}
	case "MX":
		if _, err := NormalizeDomain(record.Value); err != nil {
			return errors.New("MX value must be a domain")
		}
	default:
		return fmt.Errorf("record type %q is unsupported", record.Type)
	}
	_ = domain
	return nil
}

func renderAnswer(qname string, record Record) string {
	switch record.Type {
	case "CNAME":
		return fmt.Sprintf("%s %d IN CNAME %s.", qname, record.TTL, strings.TrimSuffix(record.Value, "."))
	case "MX":
		return fmt.Sprintf("%s %d IN MX %d %s.", qname, record.TTL, record.MXPriority, strings.TrimSuffix(record.Value, "."))
	case "TXT":
		return fmt.Sprintf("%s %d IN TXT %q", qname, record.TTL, record.Value)
	default:
		return fmt.Sprintf("%s %d IN %s %s", qname, record.TTL, record.Type, record.Value)
	}
}

func escapeAnswer(answer string) string {
	return strings.ReplaceAll(answer, `"`, `\"`)
}

func parseAnswer(domain string, answer string) (Record, error) {
	fields := strings.Fields(answer)
	if len(fields) < 5 {
		return Record{}, errors.New("answer is invalid")
	}
	qname := strings.TrimSuffix(fields[0], ".")
	suffix := "." + domain
	name := "@"
	if qname != domain {
		if !strings.HasSuffix(qname, suffix) {
			return Record{}, errors.New("answer domain mismatch")
		}
		name = strings.TrimSuffix(qname, suffix)
	}
	ttl, err := strconv.Atoi(fields[1])
	if err != nil {
		return Record{}, err
	}
	recordType := strings.ToUpper(fields[3])
	record := Record{Name: name, Type: recordType, TTL: ttl}
	switch recordType {
	case "MX":
		if len(fields) < 6 {
			return Record{}, errors.New("MX answer is invalid")
		}
		priority, err := strconv.Atoi(fields[4])
		if err != nil {
			return Record{}, err
		}
		record.MXPriority = priority
		record.Value = strings.TrimSuffix(fields[5], ".")
	case "TXT":
		value := strings.Join(fields[4:], " ")
		value = strings.Trim(value, `"`)
		record.Value = value
	default:
		record.Value = strings.TrimSuffix(fields[4], ".")
		if recordType == "A" || recordType == "AAAA" {
			record.Value = fields[4]
		}
	}
	return record, nil
}

func CollectServiceExternalIPs(ingress []string, externalIPs []string) []string {
	result := append([]string{}, externalIPs...)
	for _, item := range ingress {
		if ip := net.ParseIP(item); ip != nil {
			result = append(result, item)
			continue
		}
		if item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}
