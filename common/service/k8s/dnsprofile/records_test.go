package dnsprofile

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

type fakeRecordWriter struct {
	records []dnsRecord
	added   []string
	deleted []string
	failAt  int
}

func (w *fakeRecordWriter) List(context.Context, string, string) ([]dnsRecord, error) {
	return w.records, nil
}
func (w *fakeRecordWriter) Add(_ context.Context, _, _, kind, value string) (string, error) {
	if w.failAt > 0 && len(w.added)+1 == w.failAt {
		return "", errors.New("write failed")
	}
	w.added = append(w.added, value)
	return value, nil
}
func (w *fakeRecordWriter) Delete(_ context.Context, id string) error {
	w.deleted = append(w.deleted, id)
	return nil
}

func TestDesiredRecords(t *testing.T) {
	values, kind, err := desiredRecords(k8s.DomainParseConfigCRDSpec{IPs: []string{"1.2.3.4", "1.2.3.4", "5.6.7.8"}})
	if err != nil || kind != "A" || len(values) != 2 {
		t.Fatalf("A records: %v %q %v", values, kind, err)
	}
	values, kind, err = desiredRecords(k8s.DomainParseConfigCRDSpec{Type: "cname", Cname: "EXAMPLE.COM."})
	if err != nil || kind != "CNAME" || len(values) != 1 || values[0] != "example.com" {
		t.Fatalf("CNAME: %v %q %v", values, kind, err)
	}
	if _, _, err = desiredRecords(k8s.DomainParseConfigCRDSpec{Type: "A", IPs: []string{"::1"}}); err == nil {
		t.Fatal("accepted IPv6 as A")
	}
}

func TestEnsureRecords(t *testing.T) {
	ctx := context.Background()
	w := &fakeRecordWriter{records: []dnsRecord{{Type: "A", Value: "1.2.3.4"}}}
	if err := ensureRecords(ctx, w, "example.com", "*", "A", []string{"1.2.3.4", "5.6.7.8"}); err != nil || len(w.added) != 1 {
		t.Fatalf("add missing: %v %#v", err, w.added)
	}
	w = &fakeRecordWriter{records: []dnsRecord{{Type: "A", Value: "9.9.9.9"}}}
	if err := ensureRecords(ctx, w, "example.com", "*", "A", []string{"1.2.3.4"}); err == nil || len(w.added) != 0 {
		t.Fatalf("did not stop on conflict: %v", err)
	}
	w = &fakeRecordWriter{failAt: 2}
	if err := ensureRecords(ctx, w, "example.com", "*", "A", []string{"1.2.3.4", "5.6.7.8"}); err == nil || !strings.Contains(err.Error(), "write failed") || len(w.deleted) != 1 || w.deleted[0] != "1.2.3.4" {
		t.Fatalf("rollback: %v %#v", err, w.deleted)
	}
}
