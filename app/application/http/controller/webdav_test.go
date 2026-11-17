package controller

import "testing"

func TestChildAgentSkipsUserNamespaceIDMapping(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	if mapper := newWebDAVIDMapper("1", ""); mapper != nil {
		t.Fatal("child agent should not use uid/gid namespace mapper")
	}
	if mapper := newPermissionIDMapper("1", ""); mapper != nil {
		t.Fatal("child agent permissions should not use uid/gid namespace mapper")
	}
}
