package coredns

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAndParseZone(t *testing.T) {
	records := []Record{
		{Name: "www", Type: "A", TTL: 60, Value: "1.1.1.1"},
		{Name: "www", Type: "A", TTL: 60, Value: "2.2.2.2"},
		{Name: "www", Type: "AAAA", TTL: 120, Value: "2001:db8::1"},
		{Name: "@", Type: "MX", TTL: 300, Value: "mail.example.com", MXPriority: 20},
		{Name: "txt", Type: "TXT", TTL: 60, Value: "hello world"},
		{Name: "cdn", Type: "CNAME", TTL: 60, Value: "target.example.net"},
	}

	data, err := RenderZone("example.com", records)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "$ORIGIN example.com.") {
		t.Fatalf("expected zone origin, got %s", data)
	}
	if !strings.Contains(data, "@ IN SOA ns.example.com. admin.example.com.") {
		t.Fatalf("expected SOA, got %s", data)
	}
	if !strings.Contains(data, "www 60 IN A 1.1.1.1") {
		t.Fatalf("expected A answer, got %s", data)
	}
	if !strings.Contains(data, "@ 300 IN MX 20 mail.example.com.") {
		t.Fatalf("expected MX answer, got %s", data)
	}
	if strings.Contains(data, "template ") {
		t.Fatalf("did not expect template plugin output, got %s", data)
	}

	parsed, err := ParseZone("example.com", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(records) {
		t.Fatalf("expected %d records, got %d", len(records), len(parsed))
	}
	for _, record := range parsed {
		if record.ID == "" {
			t.Fatalf("expected record id for %#v", record)
		}
	}
}

func TestRenderZoneServer(t *testing.T) {
	data, err := RenderZoneServer("example.com")
	if err != nil {
		t.Fatal(err)
	}
	expected := "example.com {\n  file /etc/coredns/custom/example.com.zone {\n    reload 5s\n    fallthrough\n  }\n  reload\n  loadbalance\n}\n"
	if data != expected {
		t.Fatalf("unexpected server block:\n%s", data)
	}
}

func TestParseZoneSupportsApexAndSubdomainRecords(t *testing.T) {
	data := `$ORIGIN test4.com.

@ IN SOA ns.test4.com. admin.test4.com. (
    2026060201
    3600
    1800
    86400
    1
)

@ 60 IN A 8.8.8.8
a 1 IN A 10.42.0.154
b 1 IN A 1.0.0.4
`
	records, err := ParseZone("test4.com", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d: %#v", len(records), records)
	}
	if records[0].Name != "@" || records[0].Value != "8.8.8.8" {
		t.Fatalf("expected apex A record, got %#v", records[0])
	}
	if records[1].Name != "a" || records[1].Value != "10.42.0.154" {
		t.Fatalf("expected subdomain A record, got %#v", records[1])
	}
}

func TestParseLegacyTemplateZone(t *testing.T) {
	data := `test4.com {
  template IN A test4.com. {
    # w7-dns-record-id: apex-id
    answer "test4.com. 60 IN A 8.8.8.8"
  }

  template IN A a.test4.com. {
    answer "a.test4.com. 1 IN A 10.42.0.154"
  }

  template IN TXT txt.test4.com. {
    answer "txt.test4.com. 60 IN TXT \"hello world\""
  }
}
`
	records, err := ParseLegacyTemplateZone("test4.com", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d: %#v", len(records), records)
	}
	if records[0].Name != "@" || records[0].Value != "8.8.8.8" || records[0].ID != "apex-id" {
		t.Fatalf("unexpected apex record: %#v", records[0])
	}
	if records[1].Name != "a" || records[1].Value != "10.42.0.154" {
		t.Fatalf("unexpected subdomain record: %#v", records[1])
	}
	if records[2].Name != "txt" || records[2].Value != "hello world" {
		t.Fatalf("unexpected TXT record: %#v", records[2])
	}
}

func TestNextZoneSerial(t *testing.T) {
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	if serial := nextZoneSerial(0, now); serial != 2026060201 {
		t.Fatalf("unexpected initial serial: %d", serial)
	}
	if serial := nextZoneSerial(2026060201, now); serial != 2026060202 {
		t.Fatalf("unexpected incremented serial: %d", serial)
	}
	if serial := nextZoneSerial(2026060305, now); serial != 2026060306 {
		t.Fatalf("unexpected monotonic serial: %d", serial)
	}
}

func TestNormalizeRecord(t *testing.T) {
	record, err := NormalizeRecord("example.com", Record{Name: "", Type: "a", Value: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Name != "@" || record.Type != "A" || record.TTL != DefaultTTL {
		t.Fatalf("unexpected normalized record: %#v", record)
	}
}

func TestNormalizeRecordRejectsInvalidValue(t *testing.T) {
	_, err := NormalizeRecord("example.com", Record{Name: "www", Type: "A", Value: "not-an-ip"})
	if err == nil {
		t.Fatal("expected invalid A record error")
	}
}
