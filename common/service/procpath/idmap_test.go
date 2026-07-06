package procpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIDMap(t *testing.T) {
	ranges := parseIDMap(`
         0     100000      65536
     70000     200000         10
invalid line
`)
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
	if ranges[0].containerID != 0 || ranges[0].hostID != 100000 || ranges[0].size != 65536 {
		t.Fatalf("unexpected first range: %+v", ranges[0])
	}
	if ranges[1].containerID != 70000 || ranges[1].hostID != 200000 || ranges[1].size != 10 {
		t.Fatalf("unexpected second range: %+v", ranges[1])
	}
}

func TestIDMapperMapsHostAndContainerIDs(t *testing.T) {
	mapper := &IDMapper{
		uidRanges: parseIDMap("0 100000 65536\n70000 200000 10\n"),
		gidRanges: parseIDMap("0 300000 65536\n"),
	}

	if got := mapper.HostToContainerUID(100001); got != 1 {
		t.Fatalf("HostToContainerUID mismatch: got=%d want=1", got)
	}
	if got := mapper.ContainerToHostUID(70005); got != 200005 {
		t.Fatalf("ContainerToHostUID mismatch: got=%d want=200005", got)
	}
	if got := mapper.HostToContainerGID(300123); got != 123 {
		t.Fatalf("HostToContainerGID mismatch: got=%d want=123", got)
	}
	if got := mapper.ContainerToHostGID(123); got != 300123 {
		t.Fatalf("ContainerToHostGID mismatch: got=%d want=300123", got)
	}
}

func TestIDMapperFallsBackToIdentity(t *testing.T) {
	mapper := &IDMapper{
		uidRanges: parseIDMap("0 100000 10\n"),
	}

	if got := mapper.HostToContainerUID(42); got != 42 {
		t.Fatalf("HostToContainerUID fallback mismatch: got=%d want=42", got)
	}
	if got := mapper.ContainerToHostGID(42); got != 42 {
		t.Fatalf("ContainerToHostGID fallback mismatch: got=%d want=42", got)
	}
}

func TestNewIDMapperFromPathsReadsMaps(t *testing.T) {
	tmpDir := t.TempDir()
	uidMap := filepath.Join(tmpDir, "uid_map")
	gidMap := filepath.Join(tmpDir, "gid_map")
	if err := os.WriteFile(uidMap, []byte("0 100000 65536\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gidMap, []byte("0 200000 65536\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	mapper := NewIDMapperFromPaths(uidMap, gidMap)
	if got := mapper.HostToContainerUID(100010); got != 10 {
		t.Fatalf("HostToContainerUID mismatch: got=%d want=10", got)
	}
	if got := mapper.HostToContainerGID(200010); got != 10 {
		t.Fatalf("HostToContainerGID mismatch: got=%d want=10", got)
	}
}
