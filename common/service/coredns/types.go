package coredns

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	mdns "github.com/miekg/dns"
)

const (
	CoreDNSNamespace     = "kube-system"
	CoreDNSName          = "coredns"
	CoreDNSCustomName    = "coredns-custom"
	PublicDNSServiceName = "w7-coredns-public"
	DefaultTTL           = 60
	DefaultMXPriority    = 10
	soaRefresh           = 3600
	soaRetry             = 1800
	soaExpire            = 86400
	soaMinimum           = 1
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
	return renderZone(domain, records, 0, time.Now())
}

func RenderZoneWithNextSerial(domain string, records []Record, previousZone string) (string, error) {
	serial := nextZoneSerial(extractZoneSerial(previousZone), time.Now())
	return renderZone(domain, records, serial, time.Now())
}

func RenderZoneServer(domain string) (string, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	builder.WriteString(domain)
	builder.WriteString(" {\n")
	builder.WriteString("  file /etc/coredns/custom/")
	builder.WriteString(ZoneFileConfigMapKey(domain))
	builder.WriteString(" {\n")
	builder.WriteString("    reload 5s\n")
	builder.WriteString("    fallthrough\n")
	builder.WriteString("  }\n")
	builder.WriteString("  reload\n")
	builder.WriteString("  loadbalance\n")
	builder.WriteString("}\n")
	return builder.String(), nil
}

func renderZone(domain string, records []Record, serial int64, now time.Time) (string, error) {
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
	if serial <= 0 {
		serial = nextZoneSerial(0, now)
	}

	var builder strings.Builder
	builder.WriteString("$ORIGIN ")
	builder.WriteString(domain)
	builder.WriteString(".\n\n")
	builder.WriteString("@ IN SOA ns.")
	builder.WriteString(domain)
	builder.WriteString(". admin.")
	builder.WriteString(domain)
	builder.WriteString(". (\n")
	builder.WriteString("    ")
	builder.WriteString(strconv.FormatInt(serial, 10))
	builder.WriteString("\n")
	builder.WriteString("    ")
	builder.WriteString(strconv.Itoa(soaRefresh))
	builder.WriteString("\n")
	builder.WriteString("    ")
	builder.WriteString(strconv.Itoa(soaRetry))
	builder.WriteString("\n")
	builder.WriteString("    ")
	builder.WriteString(strconv.Itoa(soaExpire))
	builder.WriteString("\n")
	builder.WriteString("    ")
	builder.WriteString(strconv.Itoa(soaMinimum))
	builder.WriteString("\n")
	builder.WriteString(")\n\n")
	for _, record := range normalized {
		builder.WriteString(renderZoneRecord(record))
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func ParseZone(domain string, data string) ([]Record, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return nil, err
	}
	parser := mdns.NewZoneParser(strings.NewReader(data), domain+".", "")
	records := make([]Record, 0)
	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		record, err := recordFromRR(domain, rr)
		if err != nil {
			continue
		}
		record, err = NormalizeRecord(domain, record)
		if err != nil {
			continue
		}
		records = append(records, record)
	}
	if err := parser.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sortRecords(records)
	return records, nil
}

func ConfigMapKey(domain string) string {
	return domain + ".server"
}

func ZoneFileConfigMapKey(domain string) string {
	return domain + ".zone"
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

func renderZoneRecord(record Record) string {
	name := record.Name
	if name == "" {
		name = "@"
	}
	switch record.Type {
	case "CNAME":
		return fmt.Sprintf("%s %d IN CNAME %s.", name, record.TTL, strings.TrimSuffix(record.Value, "."))
	case "MX":
		return fmt.Sprintf("%s %d IN MX %d %s.", name, record.TTL, record.MXPriority, strings.TrimSuffix(record.Value, "."))
	case "TXT":
		return fmt.Sprintf("%s %d IN TXT %s", name, record.TTL, strconv.Quote(record.Value))
	default:
		return fmt.Sprintf("%s %d IN %s %s", name, record.TTL, record.Type, record.Value)
	}
}

func recordNameFromRR(domain string, header *mdns.RR_Header) (string, error) {
	qname := strings.TrimSuffix(strings.ToLower(header.Name), ".")
	name := "@"
	if qname != domain {
		suffix := "." + domain
		if !strings.HasSuffix(qname, suffix) {
			return "", errors.New("record domain mismatch")
		}
		name = strings.TrimSuffix(qname, suffix)
	}
	return name, nil
}

func recordFromRR(domain string, rr mdns.RR) (Record, error) {
	header := rr.Header()
	name, err := recordNameFromRR(domain, header)
	if err != nil {
		return Record{}, err
	}
	record := Record{Name: name, TTL: int(header.Ttl)}
	switch item := rr.(type) {
	case *mdns.A:
		record.Type = "A"
		record.Value = item.A.String()
	case *mdns.AAAA:
		record.Type = "AAAA"
		record.Value = item.AAAA.String()
	case *mdns.CNAME:
		record.Type = "CNAME"
		record.Value = strings.TrimSuffix(strings.ToLower(item.Target), ".")
	case *mdns.MX:
		record.Type = "MX"
		record.MXPriority = int(item.Preference)
		record.Value = strings.TrimSuffix(strings.ToLower(item.Mx), ".")
	case *mdns.TXT:
		record.Type = "TXT"
		if len(item.Txt) == 0 {
			record.Value = ""
		} else {
			record.Value = strings.Join(item.Txt, "")
		}
	default:
		return Record{}, fmt.Errorf("record type %d is unsupported", header.Rrtype)
	}
	return record, nil
}

func extractZoneSerial(data string) int64 {
	parser := mdns.NewZoneParser(strings.NewReader(data), "", "")
	for rr, ok := parser.Next(); ok; rr, ok = parser.Next() {
		if soa, ok := rr.(*mdns.SOA); ok {
			return int64(soa.Serial)
		}
	}
	return 0
}

func nextZoneSerial(previous int64, now time.Time) int64 {
	base, _ := strconv.ParseInt(now.Format("20060102")+"01", 10, 64)
	if previous >= base {
		return previous + 1
	}
	return base
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
