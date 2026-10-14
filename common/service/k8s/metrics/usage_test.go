package metrics

import "testing"

func TestParseNodeDiskUsage(t *testing.T) {
	usage, total, err := parseNodeDiskUsage([]byte(`{"node":{"fs":{"capacityBytes":100,"availableBytes":40}}}`))
	if err != nil || usage != 60 || total != 100 {
		t.Fatalf("parseNodeDiskUsage() = (%d, %d, %v), want (60, 100, nil)", usage, total, err)
	}
}
