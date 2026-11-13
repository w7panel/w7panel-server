package dnsprofile

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/w7panel/w7panel/common/service/k8s"
)

type dnsRecord struct {
	ID    string
	Type  string
	Value string
}

type recordWriter interface {
	List(context.Context, string, string) ([]dnsRecord, error)
	Add(context.Context, string, string, string, string) (string, error)
	Delete(context.Context, string) error
}

func (s Store) EnsureRecord(ctx context.Context, owner, id, host string) error {
	secret, err := s.getSecret(ctx, owner, id)
	if err != nil {
		return err
	}
	profile := profileFromSecret(secret)
	if !WildcardInZone(host, profile.Zone) {
		return errors.New("泛域名不属于该 DNS 配置")
	}
	config, err := s.SDK.GetConfigCRD(ctx, k8s.DomainParseConfigGVR, k8s.DomainParseConfigName)
	if err != nil {
		return fmt.Errorf("读取系统域名解析配置失败: %w", err)
	}
	values, recordType, err := desiredRecords(k8s.ParseDomainParseConfigCRDSpec(config))
	if err != nil {
		return err
	}
	base := strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), ".")), "*.")
	zone, err := dns01.FindZoneByFqdn(base + ".")
	if err != nil {
		return fmt.Errorf("查询 DNS 托管区域失败: %w", err)
	}
	zone = strings.TrimSuffix(zone, ".")
	if !validZone(zone) || (base != zone && !strings.HasSuffix(base, "."+zone)) {
		return errors.New("DNS 托管区域无效")
	}
	rr := "*"
	if base != zone {
		rr += "." + strings.TrimSuffix(base, "."+zone)
	}
	credentials := make(map[string]string, len(secret.Data))
	for key, value := range secret.Data {
		credentials[key] = string(value)
	}
	writer, err := newRecordWriter(profile.Provider, credentials)
	if err != nil {
		return err
	}
	return ensureRecords(ctx, writer, zone, rr, recordType, values)
}

func desiredRecords(spec k8s.DomainParseConfigCRDSpec) ([]string, string, error) {
	if spec.Type == "" || strings.EqualFold(spec.Type, "A") {
		values := make([]string, 0, len(spec.IPs))
		seen := map[string]bool{}
		for _, raw := range spec.IPs {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			ip, err := netip.ParseAddr(strings.TrimSpace(raw))
			if err != nil || !ip.Is4() {
				return nil, "", errors.New("系统配置包含无效的 A 记录地址")
			}
			value := ip.String()
			if !seen[value] {
				values = append(values, value)
				seen[value] = true
			}
		}
		if len(values) > 0 {
			return values, "A", nil
		}
	} else if strings.EqualFold(spec.Type, "cname") {
		value := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(spec.Cname), "."))
		if validZone(value) {
			return []string{value}, "CNAME", nil
		}
	}
	return nil, "", errors.New("系统尚未配置域名解析记录，请联系管理员获取")
}

func ensureRecords(ctx context.Context, writer recordWriter, zone, rr, recordType string, desired []string) error {
	existing, err := writer.List(ctx, zone, rr)
	if err != nil {
		return err
	}
	want := make(map[string]bool, len(desired))
	for _, value := range desired {
		want[value] = true
	}
	have := map[string]bool{}
	for _, record := range existing {
		if record.Type != "A" && record.Type != "CNAME" {
			continue
		}
		value := strings.ToLower(strings.TrimSuffix(record.Value, "."))
		if record.Type != recordType || !want[value] {
			return errors.New("泛域名已有不同的 A/CNAME 记录，请先手动处理")
		}
		have[value] = true
	}
	created := []string{}
	for _, value := range desired {
		if have[value] {
			continue
		}
		id, err := writer.Add(ctx, zone, rr, recordType, value)
		if err != nil {
			for _, addedID := range created {
				err = errors.Join(err, writer.Delete(ctx, addedID))
			}
			return fmt.Errorf("创建泛域名解析记录失败: %w", err)
		}
		if id == "" {
			return errors.New("DNS 平台未返回新记录 ID，可能已创建记录，请检查解析结果")
		}
		created = append(created, id)
	}
	return nil
}
