package audit

import "testing"

func TestBuildEntriesQuery(t *testing.T) {
	base := `audit_type:"login" tenant:"default"`
	got := buildEntriesQuery(base, 20, 40)
	want := `audit_type:"login" tenant:"default" | sort by (_time desc) | offset 40 | limit 20`
	if got != want {
		t.Fatalf("unexpected query:\nwant: %s\ngot:  %s", want, got)
	}
}
