package controller

import "testing"

type testOwnerIDMapper struct{}

func (testOwnerIDMapper) TryContainerToHostUID(uid uint32) (uint32, bool) {
	if uid == 999 {
		return 0, false
	}
	return uid + 100000, true
}

func (testOwnerIDMapper) TryContainerToHostGID(gid uint32) (uint32, bool) {
	if gid == 999 {
		return 0, false
	}
	return gid + 200000, true
}

func TestMapOwnerToHost(t *testing.T) {
	uid, gid, err := mapOwnerToHost(ownerInfo{uid: 33, gid: 44}, testOwnerIDMapper{})
	if err != nil {
		t.Fatal(err)
	}
	if uid != 100033 {
		t.Fatalf("uid mismatch: got=%d want=100033", uid)
	}
	if gid != 200044 {
		t.Fatalf("gid mismatch: got=%d want=200044", gid)
	}
}

func TestMapOwnerToHostSkipsUnsetIDs(t *testing.T) {
	uid, gid, err := mapOwnerToHost(ownerInfo{uid: 33, gid: -1}, testOwnerIDMapper{})
	if err != nil {
		t.Fatal(err)
	}
	if uid != 100033 {
		t.Fatalf("uid mismatch: got=%d want=100033", uid)
	}
	if gid != -1 {
		t.Fatalf("gid mismatch: got=%d want=-1", gid)
	}
}

func TestMapOwnerToHostFallsBackWithoutMapper(t *testing.T) {
	uid, gid, err := mapOwnerToHost(ownerInfo{uid: 33, gid: 44}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if uid != 33 {
		t.Fatalf("uid mismatch: got=%d want=33", uid)
	}
	if gid != 44 {
		t.Fatalf("gid mismatch: got=%d want=44", gid)
	}
}

func TestMapOwnerToHostFailsForUnmappedIDs(t *testing.T) {
	if _, _, err := mapOwnerToHost(ownerInfo{uid: 999, gid: 44}, testOwnerIDMapper{}); err == nil {
		t.Fatal("expected unmapped uid error")
	}
	if _, _, err := mapOwnerToHost(ownerInfo{uid: 33, gid: 999}, testOwnerIDMapper{}); err == nil {
		t.Fatal("expected unmapped gid error")
	}
}
