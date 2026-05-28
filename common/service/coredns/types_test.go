package coredns

import (
	"strings"
	"testing"
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
	if !strings.Contains(data, "example.com {") {
		t.Fatalf("expected zone block, got %s", data)
	}
	if !strings.Contains(data, `answer "www.example.com. 60 IN A 1.1.1.1"`) {
		t.Fatalf("expected A answer, got %s", data)
	}
	if strings.Count(data, "template IN A www.example.com.") != 1 {
		t.Fatalf("expected merged A template, got %s", data)
	}
	if !strings.Contains(data, `answer "example.com. 300 IN MX 20 mail.example.com."`) {
		t.Fatalf("expected MX answer, got %s", data)
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
