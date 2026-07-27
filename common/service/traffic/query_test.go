package traffic

import (
	"strings"
	"testing"
	"time"
)

func TestParseTimeRange(t *testing.T) {
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	got, err := ParseTimeRange("", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.End.Sub(got.Start) != time.Hour {
		t.Fatalf("default range = %s", got.End.Sub(got.Start))
	}
	_, err = ParseTimeRange(now.Add(-31*24*time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now)
	if err == nil {
		t.Fatal("expected 30 day limit error")
	}
}

func TestBuildFilterEscapesAndScopesNamespace(t *testing.T) {
	filter := buildFilter(QueryParams{
		Namespace: "tenant-a",
		Domain:    `demo.example.com\" OR *`,
		Keyword:   `/api/\"`,
	})
	if !strings.Contains(filter, `route_namespace:"tenant-a"`) {
		t.Fatalf("missing namespace scope: %s", filter)
	}
	if !strings.Contains(filter, `authority:"demo.example.com\\\" OR *"`) {
		t.Fatalf("domain is not quoted: %s", filter)
	}
}

func TestFoldRowsCombinesStatusCodes(t *testing.T) {
	rows := []map[string]any{
		{"authority": "demo.example.com", "status_code": "200", "requests": "8", "bytes_sent": "100"},
		{"authority": "demo.example.com", "status_code": "500", "requests": "2", "bytes_sent": "20"},
	}
	got := FoldRows(rows, "authority")
	if len(got) != 1 || number(got[0]["requests"]) != 10 || number(got[0]["errors"]) != 2 {
		t.Fatalf("unexpected fold result: %#v", got)
	}
	if number(got[0]["error_rate"]) != 0.2 {
		t.Fatalf("unexpected error rate: %#v", got[0]["error_rate"])
	}
}

func TestSanitizeStep(t *testing.T) {
	if got := sanitizeStep("5m | delete"); got != "5m" {
		t.Fatalf("unexpected step: %s", got)
	}
}

func TestSortRowsByTraffic(t *testing.T) {
	rows := []map[string]any{
		{"requests": 10, "bytes_sent": 1},
		{"requests": 1, "bytes_sent": 100},
	}
	SortRows(rows, "traffic")
	if number(rows[0]["bytes_sent"]) != 100 {
		t.Fatalf("unexpected order: %#v", rows)
	}
}
