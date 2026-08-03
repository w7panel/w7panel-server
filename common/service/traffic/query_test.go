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
		Namespace:  "tenant-a",
		Domain:     `demo.example.com\" OR *`,
		UpstreamIP: `10.42.0.8\" OR *`,
		Keyword:    `/api/\"`,
	})
	if !strings.Contains(filter, `route_namespace:"tenant-a"`) {
		t.Fatalf("missing namespace scope: %s", filter)
	}
	if !strings.Contains(filter, `authority:"demo.example.com\\\" OR *"`) {
		t.Fatalf("domain is not quoted: %s", filter)
	}
	if !strings.Contains(filter, `upstream_ip:"10.42.0.8\\\" OR *"`) {
		t.Fatalf("upstream IP is not quoted: %s", filter)
	}
}

func TestSearchRowsMatchesAnyFieldCaseInsensitively(t *testing.T) {
	rows := []map[string]any{
		{"pod_name": "Checkout-7d9", "upstream_ip": "10.42.0.8", "upstream_service": "checkout"},
		{"pod_name": "orders-5bc", "upstream_ip": "10.42.0.9", "upstream_service": "orders"},
	}
	if got := SearchRows(rows, "CHECKOUT", "pod_name", "upstream_ip", "upstream_service"); len(got) != 1 || got[0]["upstream_ip"] != "10.42.0.8" {
		t.Fatalf("unexpected search result: %#v", got)
	}
	if got := SearchRows(rows, "10.42.0.9", "pod_name", "upstream_ip", "upstream_service"); len(got) != 1 || got[0]["pod_name"] != "orders-5bc" {
		t.Fatalf("unexpected IP search result: %#v", got)
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

func TestBuildSeriesRows(t *testing.T) {
	rows := []map[string]any{
		{"_time": "2026-08-03T01:00:00Z", "protocol": "HTTP/1.1", "status_code": "200", "requests": "6", "bytes_received": "100", "bytes_sent": "200"},
		{"_time": "2026-08-03T01:00:00Z", "protocol": "HTTP/2", "status_code": "302", "requests": "2", "bytes_received": "20", "bytes_sent": "30"},
		{"_time": "2026-08-03T01:00:00Z", "protocol": "HTTP/3", "status_code": "500", "requests": "1", "bytes_received": "10", "bytes_sent": "40"},
		{"_time": "2026-08-03T01:00:00Z", "protocol": "TCP", "status_code": "0", "requests": "1", "bytes_received": "5", "bytes_sent": "5"},
	}
	httpsRows := []map[string]any{{"_time": "2026-08-03T01:00:00Z", "requests": "7"}}

	got := buildSeriesRows(rows, httpsRows, "5m")
	if len(got) != 1 {
		t.Fatalf("unexpected rows: %#v", got)
	}
	bucket := got[0]
	assertNumber(t, bucket, "requests_total", 10)
	assertNumber(t, bucket, "requests_http1", 6)
	assertNumber(t, bucket, "requests_http2", 2)
	assertNumber(t, bucket, "requests_http3", 1)
	assertNumber(t, bucket, "requests_https", 7)
	assertNumber(t, bucket, "traffic_bytes", 410)
	assertNumber(t, bucket, "bandwidth_bps", 410*8/300.0)
	assertNumber(t, bucket, "hits_total", 10)
	assertNumber(t, bucket, "hits_2xx", 6)
	assertNumber(t, bucket, "hits_3xx", 2)
	assertNumber(t, bucket, "hits_5xx", 1)
	assertNumber(t, bucket, "hits_other", 1)
	assertNumber(t, bucket, "hit_rate_total", 1)
	assertNumber(t, bucket, "hit_rate_2xx", .6)
	assertNumber(t, bucket, "hit_rate_other", .1)
}

func TestBuildSeriesRowsSortsBucketsAndKeepsHTTPSOverlap(t *testing.T) {
	rows := []map[string]any{
		{"_time": "2026-08-03T02:00:00Z", "protocol": "HTTP/2.0", "status_code": "404", "requests": 2},
		{"_time": "2026-08-03T01:00:00Z", "protocol": "HTTP/1.0", "status_code": "204", "requests": 1},
	}
	httpsRows := []map[string]any{{"_time": "2026-08-03T02:00:00Z", "requests": 2}}
	got := buildSeriesRows(rows, httpsRows, "30m")
	if len(got) != 2 || got[0]["_time"] != "2026-08-03T01:00:00Z" {
		t.Fatalf("buckets are not sorted: %#v", got)
	}
	assertNumber(t, got[1], "requests_total", 2)
	assertNumber(t, got[1], "requests_https", 2)
	assertNumber(t, got[1], "hits_4xx", 2)
}

func assertNumber(t *testing.T, row map[string]any, key string, want float64) {
	t.Helper()
	if got := number(row[key]); got != want {
		t.Fatalf("%s = %v, want %v; row=%#v", key, got, want, row)
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
