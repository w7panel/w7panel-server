package controller

import "testing"

type testOwnerIDMapper struct{}

func (testOwnerIDMapper) ContainerToHostUID(uid uint32) uint32 {
	return uid + 100000
}

func (testOwnerIDMapper) ContainerToHostGID(gid uint32) uint32 {
	return gid + 200000
}

func TestMapOwnerToHost(t *testing.T) {
	uid, gid := mapOwnerToHost(ownerInfo{uid: 33, gid: 44}, testOwnerIDMapper{})
	if uid != 100033 {
		t.Fatalf("uid mismatch: got=%d want=100033", uid)
	}
	if gid != 200044 {
		t.Fatalf("gid mismatch: got=%d want=200044", gid)
	}
}

func TestMapOwnerToHostSkipsUnsetIDs(t *testing.T) {
	uid, gid := mapOwnerToHost(ownerInfo{uid: 33, gid: -1}, testOwnerIDMapper{})
	if uid != 100033 {
		t.Fatalf("uid mismatch: got=%d want=100033", uid)
	}
	if gid != -1 {
		t.Fatalf("gid mismatch: got=%d want=-1", gid)
	}
}

func TestMapOwnerToHostFallsBackWithoutMapper(t *testing.T) {
	uid, gid := mapOwnerToHost(ownerInfo{uid: 33, gid: 44}, nil)
	if uid != 33 {
		t.Fatalf("uid mismatch: got=%d want=33", uid)
	}
	if gid != 44 {
		t.Fatalf("gid mismatch: got=%d want=44", gid)
	}
}
