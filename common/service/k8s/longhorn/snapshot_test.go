package longhorn

import (
	"testing"

	longhornV1beta2 "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetVolumeSnapshotsUsesActiveEngineChain(t *testing.T) {
	engineList := &longhornV1beta2.EngineList{Items: []longhornV1beta2.Engine{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "volume-e-1", Labels: map[string]string{"longhornvolume": "volume"}},
			Spec:       longhornV1beta2.EngineSpec{Active: false},
			Status: longhornV1beta2.EngineStatus{Snapshots: map[string]*longhornV1beta2.SnapshotInfo{
				"stale": {Name: "stale", Size: "99"},
			}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "volume-e-0", Labels: map[string]string{"longhornvolume": "volume"}},
			Spec:       longhornV1beta2.EngineSpec{Active: true},
			Status: longhornV1beta2.EngineStatus{Snapshots: map[string]*longhornV1beta2.SnapshotInfo{
				"volume-head": {Name: "volume-head", Size: "300"},
				"expand-20g":  {Name: "expand-20g", Size: "200", Removed: true, Created: "2026-07-29T01:00:00Z"},
				"user-snap":   {Name: "user-snap", Size: "100", UserCreated: true, Created: "2026-07-28T01:00:00Z"},
			}},
		},
	}}
	snapList := &longhornV1beta2.SnapshotList{Items: []longhornV1beta2.Snapshot{
		{ObjectMeta: metav1.ObjectMeta{Name: "user-snap"}, Spec: longhornV1beta2.SnapshotSpec{Volume: "volume"}, Status: longhornV1beta2.SnapshotStatus{Size: 100}},
	}}

	snapshots := GetVolumeSnapshots("volume", snapList, engineList)
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 engine snapshots, got %d", len(snapshots))
	}
	if snapshots[0].Name != "expand-20g" || snapshots[0].Size != 200 || !snapshots[0].Removed {
		t.Fatalf("unexpected expansion snapshot: %#v", snapshots[0])
	}
	if size := GetSnapshotSize("volume", snapList, engineList); size != 300 {
		t.Fatalf("expected snapshot size 300 without double counting, got %d", size)
	}
}

func TestGetVolumeSnapshotsFallsBackToSnapshotCRs(t *testing.T) {
	snapList := &longhornV1beta2.SnapshotList{Items: []longhornV1beta2.Snapshot{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot-a"},
			Spec:       longhornV1beta2.SnapshotSpec{Volume: "volume"},
			Status:     longhornV1beta2.SnapshotStatus{Size: 123, CreationTime: "2026-07-29T01:00:00Z", UserCreated: true},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "other"},
			Spec:       longhornV1beta2.SnapshotSpec{Volume: "other-volume"},
			Status:     longhornV1beta2.SnapshotStatus{Size: 456},
		},
	}}

	snapshots := GetVolumeSnapshots("volume", snapList, nil)
	if len(snapshots) != 1 || snapshots[0].Name != "snapshot-a" || snapshots[0].Size != 123 {
		t.Fatalf("unexpected fallback snapshots: %#v", snapshots)
	}
}
